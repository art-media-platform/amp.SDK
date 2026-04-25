package safe

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"google.golang.org/protobuf/proto"
)

// enclave is the runtime cryptographic session.
//
// Lifecycle:
//
//	OpenEnclave() -> loads SealedTome from TomeStore
//	              -> unwraps DEK via Guard
//	              -> decrypts KeyTome payload
//	              -> returns Enclave with live key index
//
//	enclave.Close() -> generates fresh DEK
//	                -> encrypts serialized KeyTome
//	                -> wraps DEK via Guard
//	                -> saves SealedTome to TomeStore
//	                -> zeros all sensitive material
//
// The key index is fully internal; callers interact via ImportKey, GenerateKey,
// FetchPubKey, DoCryptOp, and ExportSymmetricKey.  All methods are protected
// by a RWMutex.
type enclave struct {
	mu       sync.RWMutex
	store    TomeStore
	guard    Guard
	byRing   map[tag.UID]*ringIndex
	revision int64
	aad      []byte
	closed   bool
}

// ringIndex is the in-memory index for a single keyring.
// Records are kept sorted by PubKey for O(log n) lookup.
//
// A keyring may hold multiple parallel key streams distinguished by KeyType
// (e.g. a member's identity SigningKey alongside a planet-encrypt AsymmetricKey
// in a different kit).  newestByType[t] is the pub-key of the record with the
// largest TimeID among records with KeyType t.
type ringIndex struct {
	records      []*KeyPairRecord
	newestByType [KeyType_NumTypes][]byte
}

var _ Enclave = (*enclave)(nil)

// OpenEnclave starts a new cryptographic session.
//
// If the TomeStore has no existing SealedTome, a fresh empty index is created.
// The aad parameter is bound to every seal/unseal operation for domain separation.
func OpenEnclave(
	ctx context.Context,
	store TomeStore,
	guard Guard,
	aad []byte,
) (Enclave, error) {

	enc := &enclave{
		store:  store,
		guard:  guard,
		aad:    append([]byte(nil), aad...),
		byRing: make(map[tag.UID]*ringIndex),
	}

	sealed, err := store.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("safe: failed to load SealedTome: %w", err)
	}

	if sealed == nil {
		enc.revision = 1
		return enc, nil
	}

	dek, err := guard.UnwrapDEK(ctx, sealed.WrappedDEK, enc.aad)
	if err != nil {
		return nil, fmt.Errorf("safe: failed to unwrap DEK: %w", err)
	}
	defer Zero(dek)

	tomeBytes, err := OpenAEAD(dek, sealed.TomeNonce, sealed.Cipherblob, enc.aad)
	if err != nil {
		return nil, fmt.Errorf("safe: failed to decrypt KeyTome: %w", err)
	}
	defer Zero(tomeBytes)

	tome := &KeyTome{}
	if err := proto.Unmarshal(tomeBytes, tome); err != nil {
		return nil, fmt.Errorf("safe: failed to unmarshal KeyTome: %w", err)
	}

	enc.revision = tome.Revision
	for _, rec := range tome.Keys {
		enc.mergeRecord(rec)
	}
	return enc, nil
}

// ImportKey inserts a keypair into the given keyring.
func (enc *enclave) ImportKey(keyringID tag.UID, kp KeyPair) error {
	enc.mu.Lock()
	defer enc.mu.Unlock()

	if enc.closed {
		return fmt.Errorf("safe: enclave is closed")
	}
	if len(kp.Pub.Bytes) < MinPubKeyPrefixSz {
		return status.Code_BadKeyFormat.Errorf("safe: imported public key too short (got %d bytes)", len(kp.Pub.Bytes))
	}

	timeID := kp.Pub.TimeID
	if timeID.IsNil() {
		timeID = tag.NowID()
	}

	rec := &KeyPairRecord{
		KeyringID_0: keyringID[0],
		KeyringID_1: keyringID[1],
		CryptoKitID: kp.Pub.CryptoKitID,
		KeyType:     kp.Pub.KeyType,
		TimeID_0:    timeID[0],
		TimeID_1:    timeID[1],
		PubKey:      append([]byte(nil), kp.Pub.Bytes...),
		PrvKey:      append([]byte(nil), kp.Prv...),
	}
	return enc.mergeRecord(rec)
}

// GenerateKey creates a new keypair under the given keyring.
func (enc *enclave) GenerateKey(keyringID tag.UID, spec KeySpec) (PubKey, error) {
	enc.mu.Lock()
	defer enc.mu.Unlock()

	if enc.closed {
		return PubKey{}, fmt.Errorf("safe: enclave is closed")
	}

	kit, err := GetKit(spec.CryptoKitID)
	if err != nil {
		return PubKey{}, err
	}

	size := spec.RequestedSize
	if size <= 0 {
		size = DEKSize
	}

	// Natural key collisions are astronomically unlikely.
	// Retry as a safeguard against the impossible.
	for attempt := 0; attempt < 3; attempt++ {
		kp := &KeyPair{
			Pub: PubKey{
				CryptoKitID: spec.CryptoKitID,
				KeyType:     spec.KeyType,
				TimeID:      tag.NowID(),
			},
		}
		if err := generateKeyForSpec(kit, RandReader, size, kp); err != nil {
			return PubKey{}, err
		}
		if kp.Pub.CryptoKitID != spec.CryptoKitID || kp.Pub.KeyType != spec.KeyType {
			return PubKey{}, status.Code_KeyGenerationFailed.Error("safe: KitSpec mutated key spec")
		}

		rec := &KeyPairRecord{
			KeyringID_0: keyringID[0],
			KeyringID_1: keyringID[1],
			CryptoKitID: kp.Pub.CryptoKitID,
			KeyType:     kp.Pub.KeyType,
			TimeID_0:    kp.Pub.TimeID[0],
			TimeID_1:    kp.Pub.TimeID[1],
			PubKey:      kp.Pub.Bytes,
			PrvKey:      kp.Prv,
		}

		if err := enc.mergeRecord(rec); err != nil {
			if status.GetCode(err) == status.Code_BadKeyFormat {
				// pub-key collision — regenerate
				kp.Zero()
				continue
			}
			return PubKey{}, err
		}
		return pubKeyFromRecord(rec), nil
	}
	return PubKey{}, status.Code_KeyGenerationFailed.Error("safe: 3 consecutive pub-key collisions during generate")
}

// FetchPubKey returns the PubKey for the referenced entry.
func (enc *enclave) FetchPubKey(ref *KeyRef) (PubKey, error) {
	enc.mu.RLock()
	defer enc.mu.RUnlock()

	if enc.closed {
		return PubKey{}, fmt.Errorf("safe: enclave is closed")
	}

	rec, err := enc.fetchRecord(ref)
	if err != nil {
		return PubKey{}, err
	}
	return pubKeyFromRecord(rec), nil
}

// fetchTypedRecord resolves ref, enforces enclave-not-closed, and verifies that
// the resolved record's KeyType matches wantType.  Returns the record and its
// kit on success.  Internal helper for the typed Enclave methods below.
func (enc *enclave) fetchTypedRecord(ref *KeyRef, wantType KeyType, opLabel string) (*KeyPairRecord, *KitSpec, error) {
	if enc.closed {
		return nil, nil, fmt.Errorf("safe: enclave is closed")
	}
	if ref == nil {
		return nil, nil, status.Code_BadRequest.Errorf("%s: ref is required", opLabel)
	}
	rec, err := enc.fetchRecord(ref)
	if err != nil {
		return nil, nil, err
	}
	if rec.KeyType != wantType {
		return nil, nil, status.Code_BadKeyFormat.Errorf("%s requires %s, got %s", opLabel, wantType.String(), rec.KeyType.String())
	}
	kit, err := GetKit(rec.CryptoKitID)
	if err != nil {
		return nil, nil, err
	}
	return rec, kit, nil
}

// Sign produces a cryptographic signature over digest using ref's SigningKey.
func (enc *enclave) Sign(ref *KeyRef, digest []byte) ([]byte, error) {
	enc.mu.RLock()
	defer enc.mu.RUnlock()

	rec, kit, err := enc.fetchTypedRecord(ref, KeyType_SigningKey, "Sign")
	if err != nil {
		return nil, err
	}
	if kit.Signing == nil || kit.Signing.Sign == nil {
		return nil, status.Code_Unimplemented.Errorf("KitSpec %s does not support signing", kit.ID.String())
	}
	return kit.Signing.Sign(digest, rec.PrvKey)
}

// EncryptSym encrypts plaintext using ref's SymmetricKey.
// Output: nonce (24) || ciphertext+tag.  XChaCha20-Poly1305 is kit-agnostic.
func (enc *enclave) EncryptSym(ref *KeyRef, plaintext []byte) ([]byte, error) {
	enc.mu.RLock()
	defer enc.mu.RUnlock()

	rec, _, err := enc.fetchTypedRecord(ref, KeyType_SymmetricKey, "EncryptSym")
	if err != nil {
		return nil, err
	}
	nonce, ct, err := SealAEAD(RandReader, rec.PrvKey, plaintext, nil)
	if err != nil {
		return nil, err
	}
	return append(nonce, ct...), nil
}

// DecryptSym decrypts a buffer produced by EncryptSym using ref's SymmetricKey.
func (enc *enclave) DecryptSym(ref *KeyRef, ciphertext []byte) ([]byte, error) {
	enc.mu.RLock()
	defer enc.mu.RUnlock()

	rec, _, err := enc.fetchTypedRecord(ref, KeyType_SymmetricKey, "DecryptSym")
	if err != nil {
		return nil, err
	}
	if len(ciphertext) < NonceSize {
		return nil, status.Code_DecryptFailed.Error("ciphertext too short")
	}
	return OpenAEAD(rec.PrvKey, ciphertext[:NonceSize], ciphertext[NonceSize:], nil)
}

// OpenFromPub decrypts a sealed-box ciphertext using the recipient's private
// key referenced by ref.  The ephemeral sender pubkey is parsed from the front
// of msg per the kit's wire format.  ref must reference an AsymmetricKey.
func (enc *enclave) OpenFromPub(ref *KeyRef, msg []byte) ([]byte, error) {
	enc.mu.RLock()
	defer enc.mu.RUnlock()

	if enc.closed {
		return nil, fmt.Errorf("safe: enclave is closed")
	}
	if ref == nil {
		return nil, status.Code_BadRequest.Error("OpenFromPub: ref is required")
	}

	rec, err := enc.fetchRecord(ref)
	if err != nil {
		return nil, err
	}
	if rec.KeyType != KeyType_AsymmetricKey {
		return nil, status.Code_BadKeyFormat.Errorf("OpenFromPub requires AsymmetricKey, got %s", rec.KeyType.String())
	}
	kit, err := GetKit(rec.CryptoKitID)
	if err != nil {
		return nil, err
	}
	if kit.Encrypt == nil || kit.Encrypt.Open == nil {
		return nil, status.Code_Unimplemented.Errorf("KitSpec %s does not support asymmetric decryption", kit.ID.String())
	}
	return kit.Encrypt.Open(msg, rec.PrvKey)
}

func (enc *enclave) ExportSymmetricKey(ref *KeyRef) ([]byte, error) {
	enc.mu.RLock()
	defer enc.mu.RUnlock()

	if enc.closed {
		return nil, fmt.Errorf("safe: enclave is closed")
	}

	rec, err := enc.fetchRecord(ref)
	if err != nil {
		return nil, err
	}
	if rec.KeyType != KeyType_SymmetricKey {
		return nil, status.Code_BadKeyFormat.Errorf("ExportSymmetricKey: key is %s, not symmetric", rec.KeyType.String())
	}
	if len(rec.PrvKey) == 0 {
		return nil, status.Code_BadKeyFormat.Error("ExportSymmetricKey: symmetric key has no private material")
	}

	out := make([]byte, len(rec.PrvKey))
	copy(out, rec.PrvKey)
	return out, nil
}

// Close seals the current key index and persists it, then zeros sensitive material.
func (enc *enclave) Close(ctx context.Context) error {
	enc.mu.Lock()
	defer enc.mu.Unlock()

	if enc.closed {
		return nil
	}

	tome := &KeyTome{
		Revision: enc.revision,
		Keys:     enc.flattenRecords(),
	}

	tomeBytes, err := proto.Marshal(tome)
	if err != nil {
		return fmt.Errorf("safe: failed to marshal KeyTome: %w", err)
	}
	defer Zero(tomeBytes)

	dek, err := GenerateDEK(RandReader)
	if err != nil {
		return err
	}
	defer Zero(dek)

	tomeNonce, cipherblob, err := SealAEAD(RandReader, dek, tomeBytes, enc.aad)
	if err != nil {
		return fmt.Errorf("safe: failed to encrypt KeyTome: %w", err)
	}

	wrappedDEK, err := enc.guard.WrapDEK(ctx, dek, enc.aad)
	if err != nil {
		return fmt.Errorf("safe: failed to wrap DEK: %w", err)
	}

	sealed := &SealedTome{
		Version:    uint32(Const_SealedTomeVersion),
		WrappedDEK: wrappedDEK,
		Purpose:    "session",
		TomeCipher: CipherName,
		TomeNonce:  tomeNonce,
		Cipherblob: cipherblob,
	}

	if err := enc.store.Save(ctx, sealed); err != nil {
		return fmt.Errorf("safe: failed to save SealedTome: %w", err)
	}

	enc.zeroState()
	return nil
}

// mergeRecord inserts a single record into the index, rejecting mismatched
// (PubKey, KeyType) collisions.  Exact duplicates are ignored.  Same PubKey
// under different KeyTypes is permitted: a keyring may hold one identity
// SigningKey and one encrypt AsymmetricKey whose raw bytes happen to coincide
// (e.g. a temp bootstrap key imported under both types during invite onboarding).
//
// Must be called with enc.mu held.
func (enc *enclave) mergeRecord(rec *KeyPairRecord) error {
	ringID := recordKeyringID(rec)
	ring := enc.byRing[ringID]
	if ring == nil {
		ring = &ringIndex{}
		enc.byRing[ringID] = ring
	}

	pos := sort.Search(len(ring.records), func(i int) bool {
		return bytes.Compare(ring.records[i].PubKey, rec.PubKey) >= 0
	})
	for i := pos; i < len(ring.records) && bytes.Equal(ring.records[i].PubKey, rec.PubKey); i++ {
		if ring.records[i].KeyType != rec.KeyType {
			continue
		}
		if !recordsEqual(ring.records[i], rec) {
			return status.Code_BadKeyFormat.Errorf("safe: pub-key collision for keyring %s", ringID.Base32())
		}
		return nil
	}

	ring.records = append(ring.records, nil)
	copy(ring.records[pos+1:], ring.records[pos:])
	ring.records[pos] = rec

	newestTID := ringNewestTimeID(ring, rec.KeyType)
	recTID := recordTimeID(rec)
	if ring.newestByType[rec.KeyType] == nil || recTID[0] > newestTID[0] ||
		(recTID[0] == newestTID[0] && recTID[1] >= newestTID[1]) {
		ring.newestByType[rec.KeyType] = rec.PubKey
	}
	enc.revision++
	return nil
}

// fetchRecord returns the record matched by ref, or an error if none.
//
// Resolution rules:
//   - ref.Type explicit + PubKey empty: returns newest record of that type.
//   - ref.Type explicit + PubKey set:   prefix lookup, type must match.
//   - ref.Type Unspecified + PubKey empty: returns newest record across any type
//     (backward-compatible default for single-stream rings).
//   - ref.Type Unspecified + PubKey set:   prefix lookup, no type filter.
//
// Must be called with enc.mu held.
func (enc *enclave) fetchRecord(ref *KeyRef) (*KeyPairRecord, error) {
	ringID := ref.KeyringID()
	ring := enc.byRing[ringID]
	if ring == nil || len(ring.records) == 0 {
		return nil, status.Code_KeyringNotFound.Errorf("keyring %v not found", ringID)
	}
	if len(ref.PubKey) == 0 {
		if ref.Type != KeyType_Unspecified {
			pub := ring.newestByType[ref.Type]
			if len(pub) == 0 {
				return nil, status.Code_KeyringNotFound.Errorf("keyring %v has no %s keys", ringID, ref.Type.String())
			}
			if rec := ringLookupTyped(ring, pub, ref.Type); rec != nil {
				return rec, nil
			}
			return nil, status.Code_KeyringNotFound.Errorf("keyring %v: %s newest pointer stale", ringID, ref.Type.String())
		}
		bestPub, _ := ringNewestAcrossTypes(ring)
		if bestPub == nil {
			return nil, status.Code_KeyringNotFound.Errorf("keyring %v has no keys", ringID)
		}
		if rec := ringLookup(ring, bestPub); rec != nil {
			return rec, nil
		}
		return nil, status.Code_KeyringNotFound.Errorf("keyring %v: newest pointer stale", ringID)
	}
	if rec := ringLookupTypedPrefix(ring, ref.PubKey, ref.Type); rec != nil {
		return rec, nil
	}
	return nil, status.Code_KeyringNotFound.Errorf("key not found in keyring %v (pubKey prefix: %x type: %s)", ringID, ref.PubKey, ref.Type.String())
}

// ringNewestAcrossTypes returns the newest record's pub key across all KeyType
// streams, plus its TimeID. Returns (nil, zero-UID) if the ring has no keys.
func ringNewestAcrossTypes(ring *ringIndex) ([]byte, tag.UID) {
	var bestPub []byte
	var bestTID tag.UID
	for kt := KeyType(0); kt < KeyType_NumTypes; kt++ {
		pub := ring.newestByType[kt]
		if len(pub) == 0 {
			continue
		}
		rec := ringLookupTyped(ring, pub, kt)
		if rec == nil {
			continue
		}
		tid := recordTimeID(rec)
		if bestPub == nil || tid[0] > bestTID[0] || (tid[0] == bestTID[0] && tid[1] > bestTID[1]) {
			bestPub = pub
			bestTID = tid
		}
	}
	return bestPub, bestTID
}

// flattenRecords returns a sorted flat slice of all records for persistence.
// Must be called with enc.mu held.
func (enc *enclave) flattenRecords() []*KeyPairRecord {
	total := 0
	for _, ring := range enc.byRing {
		total += len(ring.records)
	}
	out := make([]*KeyPairRecord, 0, total)

	ringIDs := make([]tag.UID, 0, len(enc.byRing))
	for id := range enc.byRing {
		ringIDs = append(ringIDs, id)
	}
	sort.Slice(ringIDs, func(i, j int) bool {
		a, b := ringIDs[i], ringIDs[j]
		return a[0] < b[0] || (a[0] == b[0] && a[1] < b[1])
	})
	for _, id := range ringIDs {
		out = append(out, enc.byRing[id].records...)
	}
	return out
}

// zeroState clears all sensitive material. Must be called with enc.mu held.
func (enc *enclave) zeroState() {
	for _, ring := range enc.byRing {
		for _, rec := range ring.records {
			Zero(rec.PrvKey)
		}
	}
	enc.byRing = nil
	Zero(enc.aad)
	enc.closed = true
}

// Helpers

func recordKeyringID(rec *KeyPairRecord) tag.UID {
	return tag.UID{rec.KeyringID_0, rec.KeyringID_1}
}

func recordTimeID(rec *KeyPairRecord) tag.UID {
	return tag.UID{rec.TimeID_0, rec.TimeID_1}
}

func recordsEqual(a, b *KeyPairRecord) bool {
	return a.KeyType == b.KeyType &&
		a.CryptoKitID == b.CryptoKitID &&
		a.TimeID_0 == b.TimeID_0 &&
		a.TimeID_1 == b.TimeID_1 &&
		bytes.Equal(a.PubKey, b.PubKey) &&
		bytes.Equal(a.PrvKey, b.PrvKey)
}

func ringLookup(ring *ringIndex, pubKey []byte) *KeyPairRecord {
	pos := sort.Search(len(ring.records), func(i int) bool {
		return bytes.Compare(ring.records[i].PubKey, pubKey) >= 0
	})
	if pos < len(ring.records) && bytes.Equal(ring.records[pos].PubKey, pubKey) {
		return ring.records[pos]
	}
	return nil
}

// ringLookupTyped returns the record with both an exact PubKey match and the
// requested KeyType, scanning the small run of same-PubKey siblings.
func ringLookupTyped(ring *ringIndex, pubKey []byte, kt KeyType) *KeyPairRecord {
	pos := sort.Search(len(ring.records), func(i int) bool {
		return bytes.Compare(ring.records[i].PubKey, pubKey) >= 0
	})
	for i := pos; i < len(ring.records) && bytes.Equal(ring.records[i].PubKey, pubKey); i++ {
		if ring.records[i].KeyType == kt {
			return ring.records[i]
		}
	}
	return nil
}

// ringLookupTypedPrefix returns the first record whose PubKey starts with the
// given prefix and (if kt != Unspecified) matches the requested KeyType.
func ringLookupTypedPrefix(ring *ringIndex, prefix []byte, kt KeyType) *KeyPairRecord {
	pos := sort.Search(len(ring.records), func(i int) bool {
		return bytes.Compare(ring.records[i].PubKey, prefix) >= 0
	})
	for i := pos; i < len(ring.records); i++ {
		if !bytes.HasPrefix(ring.records[i].PubKey, prefix) {
			break
		}
		if kt == KeyType_Unspecified || ring.records[i].KeyType == kt {
			return ring.records[i]
		}
	}
	return nil
}

func ringNewestTimeID(ring *ringIndex, kt KeyType) tag.UID {
	pub := ring.newestByType[kt]
	if len(pub) == 0 {
		return tag.UID{}
	}
	rec := ringLookupTyped(ring, pub, kt)
	if rec == nil {
		return tag.UID{}
	}
	return recordTimeID(rec)
}

func pubKeyFromRecord(rec *KeyPairRecord) PubKey {
	return PubKey{
		CryptoKitID: rec.CryptoKitID,
		KeyType:     rec.KeyType,
		TimeID:      recordTimeID(rec),
		Bytes:       append([]byte(nil), rec.PubKey...),
	}
}
