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
type ringIndex struct {
	records      []*KeyPairRecord
	newestPubKey []byte // pub key of the record with the largest TimeID
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

	kit, err := GetCryptoKit(spec.CryptoKitID)
	if err != nil {
		return PubKey{}, err
	}
	if kit.GenerateKey == nil {
		return PubKey{}, status.Code_Unimplemented.Errorf("CryptoKit %s does not support key generation", spec.CryptoKitID.String())
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
		if err := kit.GenerateKey(RandReader, size, kp); err != nil {
			return PubKey{}, err
		}
		if kp.Pub.CryptoKitID != spec.CryptoKitID || kp.Pub.KeyType != spec.KeyType {
			return PubKey{}, status.Code_KeyGenerationFailed.Error("safe: CryptoKit mutated key spec")
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

// DoCryptOp performs signing, symmetric encryption/decryption, and asymmetric encryption/decryption.
func (enc *enclave) DoCryptOp(args *CryptOpArgs) (*CryptOpOut, error) {
	enc.mu.RLock()
	defer enc.mu.RUnlock()

	if enc.closed {
		return nil, fmt.Errorf("safe: enclave is closed")
	}
	if args.OpKey == nil {
		return nil, status.Code_BadRequest.Error("CryptOpArgs.OpKey is required")
	}

	rec, err := enc.fetchRecord(args.OpKey)
	if err != nil {
		return nil, err
	}

	kit, err := GetCryptoKit(rec.CryptoKitID)
	if err != nil {
		return nil, err
	}

	out := &CryptOpOut{
		OpPubKey: rec.PubKey,
	}

	switch args.Op {

	case CryptOp_Sign:
		if kit.Sign == nil {
			return nil, status.Code_Unimplemented.Errorf("CryptoKit %s does not support signing", kit.ID.String())
		}
		out.Output, err = kit.Sign(args.Input, rec.PrvKey)

	case CryptOp_EncryptSym:
		if kit.Encrypt == nil {
			return nil, status.Code_Unimplemented.Errorf("CryptoKit %s does not support symmetric encryption", kit.ID.String())
		}
		out.Output, err = kit.Encrypt(RandReader, args.Input, rec.PrvKey)

	case CryptOp_DecryptSym:
		if kit.Decrypt == nil {
			return nil, status.Code_Unimplemented.Errorf("CryptoKit %s does not support symmetric decryption", kit.ID.String())
		}
		out.Output, err = kit.Decrypt(args.Input, rec.PrvKey)

	case CryptOp_EncryptToPeer:
		if kit.EncryptFor == nil {
			return nil, status.Code_Unimplemented.Errorf("CryptoKit %s does not support asymmetric encryption", kit.ID.String())
		}
		out.Output, err = kit.EncryptFor(RandReader, args.Input, args.PeerKey, rec.PrvKey)

	case CryptOp_DecryptFromPeer:
		if kit.DecryptFrom == nil {
			return nil, status.Code_Unimplemented.Errorf("CryptoKit %s does not support asymmetric decryption", kit.ID.String())
		}
		out.Output, err = kit.DecryptFrom(args.Input, args.PeerKey, rec.PrvKey)

	default:
		return nil, status.Code_Unimplemented.Errorf("unsupported CryptOp: %v", args.Op)
	}

	if err != nil {
		return nil, err
	}
	return out, nil
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

// mergeRecord inserts a single record into the index, rejecting mismatched pub-key collisions.
// Exact duplicates are ignored. Must be called with enc.mu held.
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
	if pos < len(ring.records) && bytes.Equal(ring.records[pos].PubKey, rec.PubKey) {
		existing := ring.records[pos]
		if !recordsEqual(existing, rec) {
			return status.Code_BadKeyFormat.Errorf("safe: pub-key collision for keyring %s", ringID.Base32())
		}
		return nil
	}

	ring.records = append(ring.records, nil)
	copy(ring.records[pos+1:], ring.records[pos:])
	ring.records[pos] = rec

	newestTID := ringNewestTimeID(ring)
	recTID := recordTimeID(rec)
	if ring.newestPubKey == nil || recTID[0] > newestTID[0] ||
		(recTID[0] == newestTID[0] && recTID[1] >= newestTID[1]) {
		ring.newestPubKey = rec.PubKey
	}
	enc.revision++
	return nil
}

// fetchRecord returns the record matched by ref, or an error if none.
// Must be called with enc.mu held.
func (enc *enclave) fetchRecord(ref *KeyRef) (*KeyPairRecord, error) {
	ringID := ref.KeyringID()
	ring := enc.byRing[ringID]
	if ring == nil || len(ring.records) == 0 {
		return nil, status.Code_KeyringNotFound.Errorf("keyring %v not found", ringID)
	}
	if len(ref.PubKey) == 0 {
		if len(ring.newestPubKey) == 0 {
			return nil, status.Code_KeyringNotFound.Errorf("keyring %v has no keys", ringID)
		}
		return ringLookup(ring, ring.newestPubKey), nil
	}
	rec := ringLookupPrefix(ring, ref.PubKey)
	if rec == nil {
		return nil, status.Code_KeyringNotFound.Errorf("key not found in keyring %v (pubKey prefix: %x)", ringID, ref.PubKey)
	}
	return rec, nil
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

func ringLookupPrefix(ring *ringIndex, prefix []byte) *KeyPairRecord {
	pos := sort.Search(len(ring.records), func(i int) bool {
		return bytes.Compare(ring.records[i].PubKey, prefix) >= 0
	})
	if pos < len(ring.records) && bytes.HasPrefix(ring.records[pos].PubKey, prefix) {
		return ring.records[pos]
	}
	return nil
}

func ringNewestTimeID(ring *ringIndex) tag.UID {
	if len(ring.newestPubKey) == 0 {
		return tag.UID{}
	}
	rec := ringLookup(ring, ring.newestPubKey)
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
