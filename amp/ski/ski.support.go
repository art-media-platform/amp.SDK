package ski

import (
	"bytes"
	"fmt"
	"hash"
	"sort"
	"sync"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/bufs"
	"golang.org/x/crypto/sha3"

	"golang.org/x/crypto/blake2b"
)

const (

	// MinPubKeyPrefixSz prevents suspiciously small pub key prefixes from being used.
	MinPubKeyPrefixSz = 16
)

// HashKit is an abstraction for hash.Hash
type HashKit struct {
	HashKitID HashKitID
	Hasher    hash.Hash
	HashSz    int
}

// ByKeyringName implements sort.Interface to sort a slice of Keyrings by binary name.
type ByKeyringName []*Keyring

func (a ByKeyringName) Len() int           { return len(a) }
func (a ByKeyringName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByKeyringName) Less(i, j int) bool { return bytes.Compare(a[i].Name, a[j].Name) < 0 }

// ByNewestKey implements sort.Interface based on KeyEntry.TimeCreated
type ByNewestKey []*KeyEntry

func (a ByNewestKey) Len() int           { return len(a) }
func (a ByNewestKey) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByNewestKey) Less(i, j int) bool { return a[i].KeyInfo.TimeCreated > a[j].KeyInfo.TimeCreated }

// CompareKeyInfo fully compares two KeyInfos, sorting first by PubKey, then TimeCreated such that
//
//	ewer keys will appear first (descending TimeCreated)
//
// If 0 is returned, a and b are identical.
func CompareKeyInfo(a, b *KeyInfo) int {

	diff := bytes.Compare(a.PubKey, b.PubKey)

	// If pub keys are equal, ensure newer keys to the left
	if diff == 0 {
		diff = int(b.TimeCreated - a.TimeCreated) // Reverse time for newer keys to appear first
		if diff == 0 {
			diff = int(a.KeyForm - b.KeyForm)
			if diff == 0 {
				diff = int(a.CryptoKitID - b.CryptoKitID)
			}
		}
	}

	return diff
}

// CompareKeyEntry fully compares two KeyEntrys.
//
// If 0 is returned, a and b are identical.
func CompareKeyEntry(a, b *KeyEntry) int {

	diff := CompareKeyInfo(a.KeyInfo, b.KeyInfo)

	if diff == 0 {
		diff = bytes.Compare(a.PrivKey, b.PrivKey)
	}

	return diff
}

// ByNewestPubKey implements sort.Interface based on KeyEntry.PubKey followed by TimeCreated.
// See CompareEntries() to see sort order.
// For keys that have the same PubKey, the newer (larger TimeCreated) keys will appear first.
type ByNewestPubKey []*KeyEntry

func (a ByNewestPubKey) Len() int           { return len(a) }
func (a ByNewestPubKey) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByNewestPubKey) Less(i, j int) bool { return CompareKeyEntry(a[i], a[j]) < 0 }

// KeyTomeMgr wraps ski.KeyTome, offering threadsafe access and easy serialization.
type KeyTomeMgr struct {
	mutex   sync.RWMutex
	keyTome KeyTome
}

// NewKeyTomeMgr creates a new KeyTomeMgr
func NewKeyTomeMgr() *KeyTomeMgr {
	return &KeyTomeMgr{
		keyTome: KeyTome{
			Rev: 1,
		},
	}
}

// Clear resets this KeyHive as if NewKeyHive() was called instead, also zeroing out all private keys.
//
// THREADSAFE
func (mgr *KeyTomeMgr) Clear() {
	mgr.mutex.Lock()
	keySets := mgr.keyTome.Keyrings
	for _, keySet := range keySets {
		keySet.ZeroOut()
	}
	mgr.mutex.Unlock()
}

// FetchKey returns the first KeyEntry in the specified key set with a matching pub key prefix.
//
// If len(inPubKeyPrefix) == 0, then the newest key on the specified is returned.
//
// THREADSAFE
func (mgr *KeyTomeMgr) FetchKey(
	keyringName []byte,
	pubKeyPrefix []byte,
) (*KeyEntry, error) {

	var (
		match *KeyEntry
		err   error
	)

	mgr.mutex.RLock()

	kr := mgr.keyTome.FetchKeyring(keyringName)
	if kr == nil || len(kr.Keys) == 0 {
		err = amp.ErrCode_KeyringNotFound.Errorf("keyring %v not found", keyringName)
	} else if len(pubKeyPrefix) == 0 {
		match = kr.FetchNewestKey()
	} else {
		match = kr.FetchKeyWithPrefix(pubKeyPrefix)
	}

	mgr.mutex.RUnlock()

	if match == nil && err == nil {
		err = amp.ErrCode_KeyringNotFound.Errorf("pub key prefix %v not found in keyring %v", pubKeyPrefix, keyringName)
	}

	return match, err
}

// ExportUsingGuide -- see ski.KeyTome.ExportUsingGuide()
func (mgr *KeyTomeMgr) ExportUsingGuide(
	inGuide *KeyTome,
	inOpts ExportKeysOptions,
) ([]byte, error) {

	mgr.mutex.RLock()
	buf, err := mgr.keyTome.ExportUsingGuide(inGuide, inOpts)
	mgr.mutex.RUnlock()

	return buf, err
}

// MergeTome merges the given tome into this tome, moving entries from ioSrc.
// If there is a KeyEntry.PubKey collision, the incoming key will remain in ioSrc
//
// THREADSAFE
func (mgr *KeyTomeMgr) MergeTome(
	ioSrc *KeyTome,
) {
	mgr.mutex.RLock()
	mgr.keyTome.MergeTome(ioSrc)
	mgr.mutex.RUnlock()
}

// Marshal writes out entire state to a given buffer.
// Warning: the return buffer is not encrypted and contains private key data!
//
// THREADSAFE
func (mgr *KeyTomeMgr) Marshal() ([]byte, error) {
	mgr.mutex.RLock()
	dAtA, err := mgr.keyTome.Marshal()
	mgr.mutex.RUnlock()

	return dAtA, err
}

// Unmarshal first calls ZeroOut() on itself and then performs deserializaton.
//
// THREADSAFE
func (mgr *KeyTomeMgr) Unmarshal(dAtA []byte) error {
	mgr.mutex.Lock()
	for _, keySet := range mgr.keyTome.Keyrings {
		keySet.ZeroOut()
	}
	err := mgr.keyTome.Unmarshal(dAtA)
	mgr.mutex.Unlock()

	return err
}

// ZeroOut zeros out the private key field of each contained key and resets the length of Entries.
func (kr *Keyring) ZeroOut() {
	for _, entry := range kr.Keys {
		entry.ZeroOut()
	}

	kr.Keys = kr.Keys[:0]
	kr.NewestPubKey = nil
}

// Resort resorts this Keyring's keys for speedy searching
func (kr *Keyring) Resort() {
	sort.Sort(ByNewestPubKey(kr.Keys))
	kr.SortedByPubKey = true
}

// Optimize resorts and resets this Keyring for optimized read access.
func (kr *Keyring) Optimize() {
	kr.Resort()

	// Maintain NewestPubKey
	kr.NewestPubKey = nil
	newest := kr.FetchNewestKey()
	if newest != nil {
		kr.NewestPubKey = newest.KeyInfo.PubKey
	} else {
		kr.NewestPubKey = nil
	}
}

// DropDupes sorts this Keyring (if not already sorted) and drops all KeyEntries
//
//	that are dupes (where all contents are identical).
//
// Returns the number of dupes dropped
func (kr *Keyring) DropDupes() int {
	if !kr.SortedByPubKey {
		kr.Resort()
	}

	dupes := 0
	N := len(kr.Keys)
	for i := 1; i < N; i++ {
		if CompareKeyEntry(kr.Keys[i-1], kr.Keys[i]) == 0 {
			dupes++
		} else if dupes > 0 {
			kr.Keys[i-dupes] = kr.Keys[i]
		}
	}

	if dupes > 0 {
		kr.Keys = kr.Keys[:N-dupes]
	}

	return dupes
}

// MergeKeys is similar to MergeTome(), this consumes entries from srcKeyring and inserts them into this Keyring.
//
// Dupe keys are ignored/dropped.  If there is a pub key collision that is NOT an exact dupe (or there is
// a sketchy looking incoming KeyEntry), then the key will remain in srcKeyring.  This should be considered an error condition
// since natural collions are impossibly rare and a bad KeyEntry should never be live.
//
// Post: len(srcKeyring.Keys) == 0 if all the incoming keys were merged
func (kr *Keyring) MergeKeys(srcKeyring *Keyring) {

	srcKeyring.DropDupes()

	newest := kr.FetchNewestKey()

	var problems []*KeyEntry

	// First, detect and skip dupes
	keysToAdd := len(srcKeyring.Keys)
	for i := 0; i < keysToAdd; i++ {
		srcEntry := srcKeyring.Keys[i]
		keyInfo := srcEntry.KeyInfo

		match := kr.FetchKey(keyInfo.PubKey)
		if match != nil {

			// If a key has a matching pub key but any other field is different, this considered a collision (and so unlikely that it's basically impossible).
			if CompareKeyEntry(match, srcEntry) != 0 || len(keyInfo.PubKey) < MinPubKeyPrefixSz {
				problems = append(problems, srcEntry)
			}

			keysToAdd--
			srcKeyring.Keys[i] = srcKeyring.Keys[keysToAdd]
			i--
		} else {
			if newest == nil {
				newest = srcEntry
			} else if keyInfo.TimeCreated >= newest.KeyInfo.TimeCreated {
				newest = srcEntry
			}
		}
	}

	// Maintain the latest pub key
	if newest != nil {
		kr.NewestPubKey = newest.KeyInfo.PubKey
	} else {
		kr.NewestPubKey = nil
	}

	if keysToAdd > 0 {
		kr.Keys = append(kr.Keys, srcKeyring.Keys[:keysToAdd]...)
		kr.Resort()
	}

	srcKeyring.Keys = append(srcKeyring.Keys[:0], problems...)
	srcKeyring.Optimize()
}

// FetchKeyWithPrefix returns the KeyEntry in this Keyring with a matching pub key prefix.
//
// O(log n) if SortedByPubKey is set, O(n) otherwise.
func (kr *Keyring) FetchKeyWithPrefix(
	inPubKeyPrefix []byte,
) *KeyEntry {
	N := len(kr.Keys)
	pos := 0

	if kr.SortedByPubKey {
		pos = sort.Search(N,
			func(i int) bool {
				return bytes.Compare(kr.Keys[i].KeyInfo.PubKey, inPubKeyPrefix) >= 0
			},
		)
	}

	for ; pos < N; pos++ {
		entry := kr.Keys[pos]
		if bytes.HasPrefix(entry.KeyInfo.PubKey, inPubKeyPrefix) {
			return entry
		}
		if kr.SortedByPubKey {
			break
		}
	}

	return nil
}

// FetchKey returns the KeyEntry in this Keyring with a matching pub key.
//
// O(log n) if SortedByPubKey is set, O(n) otherwise.
func (kr *Keyring) FetchKey(
	inPubKey []byte,
) *KeyEntry {
	N := len(kr.Keys)
	pos := 0

	if kr.SortedByPubKey {
		pos = sort.Search(N,
			func(i int) bool {
				return bytes.Compare(kr.Keys[i].KeyInfo.PubKey, inPubKey) >= 0
			},
		)
	}

	for ; pos < N; pos++ {
		entry := kr.Keys[pos]
		if bytes.Equal(entry.KeyInfo.PubKey, inPubKey) {
			return entry
		}
		if kr.SortedByPubKey {
			break
		}
	}

	return nil
}

// FetchNewestKey returns the KeyEntry with the largest TimeCreated
func (kr *Keyring) FetchNewestKey() *KeyEntry {
	var newest *KeyEntry

	if len(kr.Keys) > 0 {
		if len(kr.NewestPubKey) > 0 {
			newest = kr.FetchKey(kr.NewestPubKey)
		} else {
			for _, key := range kr.Keys {
				if newest == nil {
					newest = key
				} else if key.KeyInfo.TimeCreated > newest.KeyInfo.TimeCreated {
					newest = key
				}
			}
		}
	}

	return newest
}

// ExportKeysOptions is used with ExportWithGuide()
type ExportKeysOptions uint32

const (

	//amp.ErrorOnKeyNotFound - if set, the export attempt will return an error if a given key was not found.   Otherwise, the entry is skipped/dropped.
	ErrorOnKeyNotFound = 1 << iota
)

// ZeroOut zeros out the private key field of each key in each key set
func (tome *KeyTome) ZeroOut() {
	for _, keySet := range tome.Keyrings {
		keySet.ZeroOut()
	}
}

// FetchKeyring returns the named Keyring (or nil if not found).
//
// O(log n) if SortedByName is set, O(n) otherwise.
func (tome *KeyTome) FetchKeyring(
	inKeyringName []byte,
) *Keyring {
	N := len(tome.Keyrings)
	pos := 0

	if tome.SortedByName {
		pos = sort.Search(N,
			func(i int) bool {
				return bytes.Compare(tome.Keyrings[i].Name, inKeyringName) >= 0
			},
		)
	}

	for ; pos < N; pos++ {
		kr := tome.Keyrings[pos]
		if bytes.Equal(kr.Name, inKeyringName) {
			return kr
		}
		if tome.SortedByName {
			break
		}
	}

	return nil
}

// ExportUsingGuide walks through inGuide and for each Keyring.Name + KeyEntry.PubKey match, the KeyEntry fields
//
//	are copied to a new KeyTome.  When complete, the new KeyTome is marshalled into an output buffer and returned.
//
// Note: Only Keyring.Name and KeyEntry.PubKey are used from ioGuide (other fields are ignored).
//
// Warning: since the returned buffer contains private key bytes, one should zero the result buffer after using it.
func (tome *KeyTome) ExportUsingGuide(
	inGuide *KeyTome,
	inOpts ExportKeysOptions,
) ([]byte, error) {

	outTome := &KeyTome{
		Rev:      tome.Rev,
		Keyrings: make([]*Keyring, 0, len(inGuide.Keyrings)),
	}

	for _, krGuide := range inGuide.Keyrings {
		krSrc := tome.FetchKeyring(krGuide.Name)
		if krSrc == nil {
			if (inOpts & ErrorOnKeyNotFound) != 0 {
				return nil, amp.ErrCode_KeyringNotFound.Errorf("keyring %v not found to export", krGuide.Name)
			}
		} else {

			// If the guide Keyring is empty, that means export the whole keyring
			if len(krGuide.Keys) == 0 {
				outTome.Keyrings = append(outTome.Keyrings, krSrc)
			} else {
				newkr := &Keyring{
					Name: krSrc.Name,
					Keys: make([]*KeyEntry, 0, len(krGuide.Keys)),
				}
				outTome.Keyrings = append(outTome.Keyrings, newkr)

				for _, entry := range krGuide.Keys {
					match := krSrc.FetchKey(entry.KeyInfo.PubKey)

					if match == nil {
						if (inOpts & ErrorOnKeyNotFound) != 0 {
							return nil, amp.ErrCode_KeyringNotFound.Errorf("key %v not found to export", entry.KeyInfo.PubKey)
						}
					} else {
						newkr.Keys = append(newkr.Keys, match)
					}
				}
			}
		}
	}

	return outTome.Marshal()
}

// MergeTome merges the given tome into this tome, moving entries from srcTome.
// An incoming KeyEntry that is exact duplicate is ignored/dropped.
// If there is a Keyring containing one or more rejected keys (either ill-formed or a pub key collision
// that is NOT an exact duplicate, then the problem Keyrings will remain in srcTome and should be considered an error condition.
//
// Post: len(srcTome.Keyrings) == 0 if all keys were merged.
func (tome *KeyTome) MergeTome(
	srcTome *KeyTome,
) {

	tome.Rev++

	// Ensure better Keyring search performance
	if !tome.SortedByName {
		tome.Optimize()
	}

	var problems []*Keyring

	// Merge Keyrings that already exist (to leverage a binary search)
	krToAdd := len(srcTome.Keyrings)
	for i := 0; i < krToAdd; i++ {
		krSrc := srcTome.Keyrings[i]

		krDst := tome.FetchKeyring(krSrc.Name)
		if krDst == nil {

			// For each new Keyring that we're about to add, make sure it's prim and proper (don't trust the outside world)
			krSrc.Optimize()
			continue
		}

		krDst.MergeKeys(krSrc)
		if len(krSrc.Keys) > 0 {
			problems = append(problems, krSrc)
		}

		// If we're here, keys have been absorbed into krDst so we're done w/ the current src Keyring.
		krToAdd--
		srcTome.Keyrings[i] = srcTome.Keyrings[krToAdd]
		i--
	}

	// Add the Keyrings that didn't already exist and resort
	if krToAdd > 0 {
		tome.Keyrings = append(tome.Keyrings, srcTome.Keyrings[:krToAdd]...)
		tome.Optimize()
	}

	srcTome.Keyrings = append(srcTome.Keyrings[:0], problems...)
	srcTome.Optimize()
}

// Optimize resorts all the contained Keyrings using ByKeyringName()
func (tome *KeyTome) Optimize() {
	sort.Sort(ByKeyringName(tome.Keyrings))
	tome.SortedByName = true
}

/*
// GenerateFork returns a new KeyTome identical to this KeyTome, but with newly generated PubKey/PrivKey pairs.
// For each generated key, each originating KeyEntry's fields are reset (except for PrivKey which is set to to nil)
func (tome *KeyTome) GenerateFork(
	ioRand           io.Reader,
	inRequestedKeySz int,
) (*KeyTome, error) {

	tome.Rev++

	timeCreated := device.TimeNowFS()

	var (
		err      error
		curKit   CryptoKit
		curKitID CryptoKitID
	)

	newTome := &KeyTome{
		Rev:      1,
		Keyrings: make([]*Keyring, 0, len(tome.Keyrings)),
	}

	for _, krSrc := range tome.Keyrings {
		krDst := &Keyring{
			Name: krSrc.Name,
			Keys: make([]*KeyEntry, len(krSrc.Keys)),
		}
		newTome.Keyrings = append(newTome.Keyrings, krDst)

		for i, srcEntry := range krSrc.Keys {
			srcInfo := srcEntry.KeyInfo

			if curKitID != srcInfo.CryptoKitID {
				curKit, err = GetCryptoKit(srcInfo.CryptoKitID)
				if err != nil {
					return nil, err
				}
				curKitID = curKit.CryptoKitID()
			}

			newEntry := &KeyEntry{
				KeyInfo: &KeyInfo{
					KeyForm:     srcInfo.KeyForm,
					CryptoKitID: curKitID,
					TimeCreated: int64(timeCreated),
				},
			}

			err = curKit.GenerateNewKey(
				inRequestedKeySz,
				ioRand,
				newEntry,
			)
			if err != nil {
				return nil, err
			}
			if srcInfo.KeyForm != newEntry.KeyInfo.KeyForm || curKitID != newEntry.KeyInfo.CryptoKitID {
				return nil,amp.ErrCode_KeyGenerationFailed.Error("generate key altered key type")
			}

			krDst.Keys[i] = newEntry

			*srcInfo = *newEntry.KeyInfo
		}
	}

	return newTome, nil
}
*/

// EqualTo compares if two key entries are identical/interchangable
func (entry *KeyEntry) EqualTo(other *KeyEntry) bool {
	a := entry.KeyInfo
	b := entry.KeyInfo

	return a.KeyForm != b.KeyForm ||
		a.CryptoKitID != b.CryptoKitID ||
		a.TimeCreated != b.TimeCreated ||
		!bytes.Equal(a.PubKey, b.PubKey) ||
		!bytes.Equal(entry.PrivKey, other.PrivKey)

}

// ZeroOut zeros out this entry's private key buffer
func (entry *KeyEntry) ZeroOut() {
	N := int32(len(entry.PrivKey))
	for i := int32(0); i < N; i++ {
		entry.PrivKey[i] = 0
	}
}

// DescStr returns a human readable desc string for this KeyRef
func (kref *KeyRef) DescStr() string {
	return fmt.Sprintf("pubkey %s on keyring %s", bufs.BufDesc(kref.PubKey), bufs.BufDesc(kref.KeyringName))
}

// DescStr returns a human readable desc string for this KeyInfo
func (ki *KeyInfo) DescStr(verbose bool) string {
	if verbose {
		return fmt.Sprint("pubkey ", bufs.Base32Encoding.EncodeToString(ki.PubKey), " using ", ki.CryptoKitID.String())
	}
	return fmt.Sprint("pubkey ", bufs.BufDesc(ki.PubKey))
}

// Zero zeros out a given slice
func Zero(buf []byte) {
	N := int32(len(buf))
	for i := int32(0); i < N; i++ {
		buf[i] = 0
	}
}

// FetchHasher returns the hash pkg for the given hash kit
func FetchHasher(hashKitID HashKitID) func() hash.Hash {

	var hasher func() hash.Hash

	switch hashKitID {

	case HashKitID_LegacyKeccak_256:
		hasher = sha3.NewLegacyKeccak256

	case HashKitID_LegacyKeccak_512:
		hasher = sha3.NewLegacyKeccak512

	case HashKitID_SHA3_256:
		hasher = sha3.New256

	case HashKitID_SHA3_512:
		hasher = sha3.New512

	case HashKitID_Blake2b_256:
		hasher = func() hash.Hash {
			inst, _ := blake2b.New256(nil)
			return inst
		}

	case HashKitID_Blake2b_512:
		hasher = func() hash.Hash {
			inst, _ := blake2b.New512(nil)
			return inst
		}
	}

	return hasher
}

// NewHashKit returns the requested HashKit.
func NewHashKit(hashKitID HashKitID) (HashKit, error) {

	var kit HashKit

	if hashKitID == 0 {
		hashKitID = HashKitID_LegacyKeccak_256
	}

	hasher := FetchHasher(hashKitID)
	if hasher == nil {
		return kit, amp.ErrCode_HashKitNotFound.Errorf("unrecognized HashKitID %v", hashKitID)
	}

	kit.HashKitID = hashKitID
	kit.Hasher = hasher()
	kit.HashSz = kit.Hasher.Size()

	return kit, nil
}

/*
// GenerateNewKeys is a convenience bulk function for CryptoKit.GenerateNewKey()
func GenerateNewKeys(
	inRand io.Reader,
	inRequestedKeySz int,
	inKeySpecs []*KeyEntry,
) ([]*KeyEntry, error) {

	N := len(inKeySpecs)

	newKeys := make([]*KeyEntry, N)

	var kit *CryptoKit
	var err error

	timeCreated := plan.Now().UnixSecs

	for i, keySpec := range inKeySpecs {

		if kit == nil || kit.CryptoKit != keySpec.CryptoKit {
			kit, err = GetCryptoKit(keySpec.CryptoKit)
			if err != nil {
				return nil, err
			}
		}

		newKey := &KeyEntry{
			KeyForm:     keySpec.KeyForm,
			CryptoKit: kit.CryptoKit,
			TimeCreated: timeCreated,
		}

		err = kit.GenerateNewKey(
			inRand,
			inRequestedKeySz,
			newKey,
		)
		if err != nil {
			return nil, err
		}

		newKeys[i] = newKey
	}

	return newKeys, nil
}
*/

// GenerateNewKey creates a new key, blocking until completion
func GenerateNewKey(
	inSession EnclaveSession,
	inKeyringName []byte,
	inKeyInfo KeyInfo,
) (*KeyInfo, error) {

	tomeOut, err := inSession.GenerateKeys(&KeyTome{
		Keyrings: []*Keyring{
			{
				Name: inKeyringName,
				Keys: []*KeyEntry{
					{
						KeyInfo: &inKeyInfo,
					},
				},
			},
		},
	})

	if err != nil {
		return nil, err
	}

	var kr *Keyring
	if tomeOut != nil && tomeOut.Keyrings[0] != nil {
		kr = tomeOut.Keyrings[0]
	}

	if kr == nil || kr.Keys[0] == nil || kr.Keys[0].KeyInfo == nil {
		return nil, amp.ErrCode_AssertFailed.Error("no keys returned")
	}

	if kr.Keys[0].KeyInfo.KeyForm != inKeyInfo.KeyForm {
		return nil, amp.ErrCode_AssertFailed.Error("unexpected key type")
	}

	if !bytes.Equal(inKeyringName, kr.Name) {
		return nil, amp.ErrCode_AssertFailed.Error("generate returned different keyring name")
	}

	return kr.Keys[0].KeyInfo, nil

}

// PayloadPacker signs and packs payload buffers IAW ski.SigHeader
type PayloadPacker struct {
	signSession   EnclaveSession
	hashKit       HashKit
	signingKeyRef *KeyRef
	signingKey    KeyInfo
	threadsafe    bool
	mutex         sync.Mutex
}

// PackingInfo returns info from PackAndSign()
type PackingInfo struct {
	SignedBuf []byte
	Hash      []byte
	Sig       []byte
	Extra     []byte
}

// NewPacker creates a new PayloadSigner
func NewPacker(
	inMakeThreadsafe bool,
) PayloadPacker {

	return PayloadPacker{
		threadsafe:    inMakeThreadsafe,
		signingKeyRef: &KeyRef{},
	}
}

// ResetSession prepares this packer for use.
func (P *PayloadPacker) ResetSession(
	inSession EnclaveSession,
	inSigningKey KeyRef,
	inHashKit HashKitID,
	outKeyInfo *KeyInfo,
) error {

	P.signSession = nil
	*P.signingKeyRef = KeyRef{}
	P.signingKey = KeyInfo{}

	keyEntry, err := inSession.FetchKeyInfo(&inSigningKey)
	if err != nil {
		return err
	}
	if keyEntry.KeyForm != KeyForm_SigningKey {
		return amp.ErrCode_SessionNotReady.Error("not a signing key")
	}

	P.hashKit, err = NewHashKit(inHashKit)
	if err != nil {
		return err
	}

	P.signSession = inSession

	P.signingKeyRef.KeyringName = inSigningKey.KeyringName
	P.signingKeyRef.PubKey = keyEntry.PubKey
	P.signingKey = *keyEntry

	if outKeyInfo != nil {
		*outKeyInfo = P.signingKey
	}

	err = P.checkReady()
	if err != nil {
		return err
	}

	return nil
}

// PackAndSign signs a hash digest and packages it along with the payload and encodinginto into a single composite buffer
// intended to be decoded via PayloadUnpacker.UnpackAndVerify()
//
// THREADSAFE
func (P *PayloadPacker) PackAndSign(
	inHeaderCodec uint32,
	inHeader []byte,
	inBody []byte,
	inExtraAlloc int,
	out *PackingInfo,
) error {

	err := P.checkReady()
	if err != nil {
		return err
	}

	hdr := SigHeader{
		SignerCryptoKit: P.signingKey.CryptoKitID,
		SignerPubKey:    P.signingKey.PubKey,
		HashKitID:       P.hashKit.HashKitID,
		HeaderCodec:     uint32(inHeaderCodec),
		HeaderSz:        uint32(len(inHeader)),
		BodySz:          uint64(len(inBody)),
	}

	extra := inExtraAlloc + P.hashKit.HashSz
	bufSz := 2 + hdr.Size() + int(hdr.HeaderSz) + int(hdr.BodySz) + 100 + extra
	buf := make([]byte, bufSz)

	// 1) Marshal the SigHeader and write its length
	sigHdrSz := 0
	sigHdrSz, err = hdr.MarshalTo(buf[2:])
	if err != nil {
		return amp.ErrCode_MarshalFailed.Errorf("failed to marshal SigHeader")
	}
	pos := 2 + uint64(sigHdrSz)
	buf[0] = byte(sigHdrSz)
	buf[1] = byte(sigHdrSz >> 8)

	// 2) Write the payload header next
	copy(buf[pos:], inHeader)
	pos += uint64(hdr.HeaderSz)

	// 3) Write the payload body
	copy(buf[pos:], inBody)
	pos += hdr.BodySz

	if P.threadsafe {
		P.mutex.Lock()
	}

	// 4) Hash the buf so far
	P.hashKit.Hasher.Reset()
	P.hashKit.Hasher.Write(buf[:pos])
	extraPos := bufSz - extra
	out.Hash = P.hashKit.Hasher.Sum(buf[extraPos:extraPos])
	out.Extra = buf[extraPos+P.hashKit.HashSz:]

	if P.threadsafe {
		P.mutex.Unlock()
	}

	if len(out.Hash) != P.hashKit.HashSz {
		return amp.ErrCode_AssertFailed.Error("hasher returned bad digest length")
	}

	// 5) Sign the hash
	signOut, err := P.signSession.DoCryptOp(&CryptOpArgs{
		CryptOp: CryptOp_Sign,
		BufIn:   out.Hash,
		OpKey:   P.signingKeyRef,
	})
	if err != nil {
		return err
	}

	// 6) Append the sig len
	sigLen := uint64(len(signOut.BufOut))
	if sigLen > 255 {
		return amp.ErrCode_AssertFailed.Error("unexpected sig length")
	}
	buf[pos] = byte(sigLen)
	pos++

	// 6) Append the sig
	copy(buf[pos:], signOut.BufOut)
	out.Sig = buf[pos : pos+sigLen]
	pos += sigLen

	out.SignedBuf = buf[:pos]

	return nil
}

// checkReady checks if everything is in place to perform SignAndPack().
//
// THREADSAFE
func (P *PayloadPacker) checkReady() error {

	if P.hashKit.Hasher == nil {
		return amp.ErrCode_SessionNotReady.Error("payload hasher not set")
	}

	if P.signSession == nil {
		return amp.ErrCode_SessionNotReady.Error("SKI signing session missing")
	}

	if len(P.signingKeyRef.KeyringName) == 0 {
		return amp.ErrCode_SessionNotReady.Error("signer keyring name missing")
	}

	if P.signingKey.CryptoKitID == 0 {
		return amp.ErrCode_SessionNotReady.Error("signing key CryptoKit not set")
	}

	return nil
}

// SignedPayload are the params associated with signing a payload buffer.
type SignedPayload struct {
	HeaderCodec uint32    // Client-specified encoding
	Header      []byte    // Client payload (hashed into .HeaderSig)
	Body        []byte    // Client body (NOT hashed into sig)
	HashKit     HashKitID // The ID of the hash kit that generated .Hash
	Hash        []byte    // A hash digest generated from .Header and .Body
	HashSig     []byte    // Signature of .Hash by .Signer
	Signer      KeyInfo   // The pub key that orginated .Sig
}

// PayloadUnpacker unpacks and decodes signed buffers IAW ski.SigHeader
type PayloadUnpacker struct {
	threadsafe bool
	mutex      sync.Mutex
	hashKits   map[HashKitID]HashKit
}

// NewUnpacker creates a new
func NewUnpacker(
	makeThreadsafe bool,
) PayloadUnpacker {
	return PayloadUnpacker{
		threadsafe: makeThreadsafe,
		hashKits:   map[HashKitID]HashKit{},
	}
}

// UnpackAndVerify decodes the given buffer into its payload and signature components, and verifies the signature.
// This procedure assumes the signed buf was produced via Signer.SignAndPack()
// Note: the returned payload buffer is a slice of inSignedBuf.
func (U *PayloadUnpacker) UnpackAndVerify(
	inSignedBuf []byte,
	out *SignedPayload,
) error {

	var (
		err error
		hdr SigHeader
	)

	sigHdrEnd := uint32(0)

	bufLen := uint32(len(inSignedBuf))
	if bufLen < 50 {
		err = amp.ErrCode_UnmarshalFailed.Errorf("signed buf is too small (len=%v)", bufLen)
	}

	// 1) Unmarshal the SigHeader info
	if err == nil {
		sigHdrEnd = 2 + uint32(inSignedBuf[0]) | (uint32(inSignedBuf[1]) << 8)
		if sigHdrEnd > bufLen-10 {
			err = amp.ErrCode_UnmarshalFailed.Error("payload pos exceeds buf size")
		}
	}

	if err == nil {
		err = hdr.Unmarshal(inSignedBuf[2:sigHdrEnd])
		if err != nil {
			err = amp.ErrCode_UnmarshalFailed.Error("failed to unmarshal ski.SigHeader")
		}
	}

	headerPos := sigHdrEnd
	headerEnd := headerPos + hdr.HeaderSz

	bodyPos := headerEnd
	bodyEnd := bodyPos + uint32(hdr.BodySz)

	if err == nil {
		if bodyEnd > bufLen-1 {
			err = amp.ErrCode_UnmarshalFailed.Error("body end exceeds buf size")
		}
	}

	// 2) Read the sig len
	sigPos := bodyEnd + 1
	sigEnd := sigPos + uint32(inSignedBuf[bodyEnd])

	if err == nil {
		if U.threadsafe {
			U.mutex.Lock()
		}

		// 3) Prep the hasher so we can generate a digest
		hashKit, ok := U.hashKits[hdr.HashKitID]
		if !ok {
			hashKit, err = NewHashKit(hdr.HashKitID)
			if err == nil {
				U.hashKits[hdr.HashKitID] = hashKit
			}
		}

		// 4) Calculate the hash digest of the header before unmarshalling further
		if err == nil {
			hashKit.Hasher.Reset()
			hashKit.Hasher.Write(inSignedBuf[:bodyEnd])
			out.Hash = hashKit.Hasher.Sum(out.Hash[:0])
		}

		if U.threadsafe {
			U.mutex.Unlock()
		}
	}

	// 5) Verify the sig
	if err == nil {
		err = VerifySignature(hdr.SignerCryptoKit, inSignedBuf[sigPos:sigEnd], out.Hash, hdr.SignerPubKey)
	}

	out.Header = inSignedBuf[headerPos:headerEnd]
	out.HeaderCodec = hdr.HeaderCodec
	out.HashKit = hdr.HashKitID
	out.HashSig = inSignedBuf[sigPos:sigEnd]
	out.Body = inSignedBuf[bodyPos:bodyEnd]
	out.Signer.PubKey = hdr.SignerPubKey
	out.Signer.KeyForm = KeyForm_SigningKey
	out.Signer.CryptoKitID = hdr.SignerCryptoKit

	return err
}
