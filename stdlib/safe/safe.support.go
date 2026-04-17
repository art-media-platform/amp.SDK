package safe

import (
	"bytes"
	"fmt"
	"hash"
	"io"
	"sort"

	"github.com/art-media-platform/amp.SDK/stdlib/data"
	"github.com/art-media-platform/amp.SDK/stdlib/encode"
	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"golang.org/x/crypto/sha3"

	"golang.org/x/crypto/blake2s"
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

// ByKeyringID implements sort.Interface to sort a slice of Keyrings by binary name.
type ByKeyringID []*Keyring

func (a ByKeyringID) Len() int      { return len(a) }
func (a ByKeyringID) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByKeyringID) Less(i, j int) bool {
	return (a[i].UID_0 < a[j].UID_0) || (a[i].UID_0 == a[j].UID_0 && a[i].UID_1 < a[j].UID_1)
}

// TimeID returns the TimeID as a tag.UID.
func (ki *KeyInfo) TimeID() tag.UID {
	return tag.UID{ki.TimeID_0, ki.TimeID_1}
}

// SetTimeID sets the TimeID fields from a tag.UID.
func (ki *KeyInfo) SetTimeID(uid tag.UID) {
	ki.TimeID_0 = uid[0]
	ki.TimeID_1 = uid[1]
}

// ContainerID returns the ContainerID as a tag.UID.
func (ek *EpochKeyEntry) ContainerID() tag.UID {
	return tag.UID{ek.ContainerID_0, ek.ContainerID_1}
}

// EpochID returns the EpochID as a tag.UID.
func (ek *EpochKeyEntry) EpochID() tag.UID {
	return tag.UID{ek.EpochID_0, ek.EpochID_1}
}

// CompareKeyInfo fully compares two KeyInfos, sorting first by PubKey, then TimeID such that
//
//	newer keys will appear first (descending TimeID)
//
// If 0 is returned, a and b are identical.
func CompareKeyInfo(a, b *KeyInfo) int {

	diff := bytes.Compare(a.PubKey, b.PubKey)

	// If pub keys are equal, ensure newer keys to the left (descending TimeID)
	if diff == 0 {
		aID := a.TimeID()
		bID := b.TimeID()
		if bID[0] > aID[0] || (bID[0] == aID[0] && bID[1] > aID[1]) {
			diff = 1
		} else if aID[0] > bID[0] || (aID[0] == bID[0] && aID[1] > bID[1]) {
			diff = -1
		}
		if diff == 0 {
			diff = int(a.KeyType - b.KeyType)
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

// ByNewestPubKey implements sort.Interface based on KeyEntry.PubKey followed by TimeID.
// See CompareEntries() to see sort order.
// For keys that have the same PubKey, the newer (larger TimeID) keys will appear first.
type ByNewestPubKey []*KeyEntry

func (a ByNewestPubKey) Len() int           { return len(a) }
func (a ByNewestPubKey) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByNewestPubKey) Less(i, j int) bool { return CompareKeyEntry(a[i], a[j]) < 0 }

func (kr *Keyring) ID() tag.UID {
	return tag.UID{
		kr.UID_0,
		kr.UID_1,
	}
}

// ZeroOut zeros out the private key field of each contained key and resets the length of Entries.
func (kr *Keyring) ZeroOut() {
	for _, entry := range kr.Keys {
		entry.ZeroOut()
	}

	kr.Keys = kr.Keys[:0]
	kr.NewestPubKey = nil
}

// Resort resorts this Keyring's keys by PubKey for speedy searching.
func (kr *Keyring) Resort(order Ordering) {
	switch order {
	case Ordering_PubKey:
		sort.Sort(ByNewestPubKey(kr.Keys))
	}

	kr.Ordering = order

}

// Optimize resorts and resets this Keyring for optimized read access.
func (kr *Keyring) Optimize() {
	kr.Resort(Ordering_PubKey)

	// Maintain NewestPubKey
	kr.NewestPubKey = nil
	newest := kr.FetchNewestKey()
	if newest != nil {
		kr.NewestPubKey = newest.KeyInfo.PubKey
	}
}

// ensureSorted sorts the Keyring if not already sorted by PubKey.
func (kr *Keyring) ensureSorted() {
	if kr.Ordering != Ordering_PubKey {
		kr.Resort(Ordering_PubKey)
	}
}

// DropDupes sorts this Keyring (if not already sorted) and drops all KeyEntries
//
//	that are dupes (where all contents are identical).
//
// Returns the number of dupes dropped
func (kr *Keyring) DropDupes() int {
	kr.ensureSorted()

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
			} else {
				srcID := keyInfo.TimeID()
				curID := newest.KeyInfo.TimeID()
				if srcID[0] > curID[0] || (srcID[0] == curID[0] && srcID[1] >= curID[1]) {
					newest = srcEntry
				}
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
		kr.Resort(Ordering_PubKey)
	}

	srcKeyring.Keys = append(srcKeyring.Keys[:0], problems...)
	srcKeyring.Optimize()
}

// FetchKeyWithPrefix returns the KeyEntry in this Keyring with a matching pub key prefix.
//
// Sorts on demand if Ordering is not PubKey, then performs O(log n) search.
func (kr *Keyring) FetchKeyWithPrefix(
	inPubKeyPrefix []byte,
) *KeyEntry {
	kr.ensureSorted()

	N := len(kr.Keys)
	pos := sort.Search(N,
		func(i int) bool {
			return bytes.Compare(kr.Keys[i].KeyInfo.PubKey, inPubKeyPrefix) >= 0
		},
	)

	if pos < N {
		entry := kr.Keys[pos]
		if bytes.HasPrefix(entry.KeyInfo.PubKey, inPubKeyPrefix) {
			return entry
		}
	}

	return nil
}

// FetchKey returns the KeyEntry in this Keyring with a matching pub key.
//
// Sorts on demand if Ordering is not PubKey, then performs O(log n) search.
func (kr *Keyring) FetchKey(
	inPubKey []byte,
) *KeyEntry {
	kr.ensureSorted()

	N := len(kr.Keys)
	pos := sort.Search(N,
		func(i int) bool {
			return bytes.Compare(kr.Keys[i].KeyInfo.PubKey, inPubKey) >= 0
		},
	)

	if pos < N {
		entry := kr.Keys[pos]
		if bytes.Equal(entry.KeyInfo.PubKey, inPubKey) {
			return entry
		}
	}

	return nil
}

// FetchNewestKey returns the KeyEntry with the largest TimeID.
func (kr *Keyring) FetchNewestKey() *KeyEntry {
	var newest *KeyEntry

	if len(kr.Keys) > 0 {
		if len(kr.NewestPubKey) > 0 {
			newest = kr.FetchKey(kr.NewestPubKey)
		}
		if newest == nil {
			for _, key := range kr.Keys {
				if newest == nil {
					newest = key
				} else {
					keyID := key.KeyInfo.TimeID()
					curID := newest.KeyInfo.TimeID()
					if keyID[0] > curID[0] || (keyID[0] == curID[0] && keyID[1] > curID[1]) {
						newest = key
					}
				}
			}
		}
	}

	return newest
}

// ExportKeysOptions is used with ExportWithGuide()
type ExportKeysOptions uint32

const (

	//status.Code_ErrorOnKeyNotFound - if set, the export attempt will return an error if a given key was not found.   Otherwise, the entry is skipped/dropped.
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
// Sorts on demand if Ordering is not KeyringID, then performs O(log n) search.
func (tome *KeyTome) FetchKeyring(
	keyringID tag.UID,
) *Keyring {
	tome.ensureSorted()

	N := len(tome.Keyrings)
	pos := sort.Search(N,
		func(i int) bool {
			return tome.Keyrings[i].UID_0 > keyringID[0] || (tome.Keyrings[i].UID_0 == keyringID[0] && tome.Keyrings[i].UID_1 >= keyringID[1])
		},
	)

	if pos < N {
		kr := tome.Keyrings[pos]
		if kr.ID() == keyringID {
			return kr
		}
	}

	return nil
}

// ensureSorted sorts the KeyTome's Keyrings by KeyringID if not already sorted.
func (tome *KeyTome) ensureSorted() {
	if tome.Ordering != Ordering_KeyringID {
		tome.Optimize()
	}
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
		Revision: tome.Revision,
		Keyrings: make([]*Keyring, 0, len(inGuide.Keyrings)),
	}

	for _, krGuide := range inGuide.Keyrings {
		krSrc := tome.FetchKeyring(krGuide.ID())
		if krSrc == nil {
			if (inOpts & ErrorOnKeyNotFound) != 0 {
				return nil, status.Code_KeyringNotFound.Errorf("keyring %v not found to export", krGuide.ID())
			}
		} else {

			// If the guide Keyring is empty, that means export the whole keyring
			if len(krGuide.Keys) == 0 {
				outTome.Keyrings = append(outTome.Keyrings, krSrc)
			} else {
				newkr := &Keyring{
					UID_0: krSrc.UID_0,
					UID_1: krSrc.UID_1,
					Keys:  make([]*KeyEntry, 0, len(krGuide.Keys)),
				}
				outTome.Keyrings = append(outTome.Keyrings, newkr)

				for _, entry := range krGuide.Keys {
					match := krSrc.FetchKey(entry.KeyInfo.PubKey)

					if match == nil {
						if (inOpts & ErrorOnKeyNotFound) != 0 {
							return nil, status.Code_KeyringNotFound.Errorf("key %v not found to export", entry.KeyInfo.PubKey)
						}
					} else {
						newkr.Keys = append(newkr.Keys, match)
					}
				}
			}
		}
	}

	return data.MarshalTo(nil, outTome)
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

	tome.Revision++

	// Ensure search performance via sort
	tome.ensureSorted()

	var problems []*Keyring

	// Merge Keyrings that already exist (to leverage a binary search)
	krToAdd := len(srcTome.Keyrings)
	for i := 0; i < krToAdd; i++ {
		krSrc := srcTome.Keyrings[i]

		krDst := tome.FetchKeyring(krSrc.ID())
		if krDst == nil {

			// For each new Keyring that we're about to add, ensure valid and well-formed (do not trust external input)
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

// Optimize resorts all the contained Keyrings using ByKeyringID()
func (tome *KeyTome) Optimize() {
	sort.Sort(ByKeyringID(tome.Keyrings))
	tome.Ordering = Ordering_KeyringID
}

// GenerateFork returns a new KeyTome identical to this KeyTome, but with newly generated PubKey/PrivKey pairs.
// For each generated key, each originating KeyEntry's fields are reset (except for PrivKey which is set to to nil)
func (tome *KeyTome) GenerateFork(
	inRand io.Reader,
	inRequestedKeySz int,
) (*KeyTome, error) {

	tome.Revision++

	var (
		err      error
		curKit   *CryptoKit
		curKitID CryptoKitID
	)

	newTome := &KeyTome{
		Revision: 1,
		Keyrings: make([]*Keyring, 0, len(tome.Keyrings)),
	}

	for _, krSrc := range tome.Keyrings {
		krDst := &Keyring{
			UID_0: krSrc.UID_0,
			UID_1: krSrc.UID_1,
			Keys:  make([]*KeyEntry, len(krSrc.Keys)),
		}
		newTome.Keyrings = append(newTome.Keyrings, krDst)

		for i, srcEntry := range krSrc.Keys {
			srcInfo := srcEntry.KeyInfo

			if curKitID != srcInfo.CryptoKitID {
				curKit, err = GetCryptoKit(srcInfo.CryptoKitID)
				if err != nil {
					return nil, err
				}
				curKitID = curKit.ID
			}

			if curKit.GenerateKey == nil {
				return nil, status.Code_Unimplemented.Errorf("CryptoKit %s does not support key generation", curKitID.String())
			}

			timeID := tag.NowID()
			newEntry := &KeyEntry{
				KeyInfo: &KeyInfo{
					KeyType:     srcInfo.KeyType,
					CryptoKitID: curKitID,
					TimeID_0:    timeID[0],
					TimeID_1:    timeID[1],
				},
			}

			err = curKit.GenerateKey(
				inRand,
				inRequestedKeySz,
				newEntry,
			)
			if err != nil {
				return nil, err
			}
			if srcInfo.KeyType != newEntry.KeyInfo.KeyType || curKitID != newEntry.KeyInfo.CryptoKitID {
				return nil, status.Code_KeyGenerationFailed.Error("generate key altered key type")
			}

			krDst.Keys[i] = newEntry

			// Update the source entry with the new public info (PrivKey stays nil in the "guide" copy)
			srcInfo.PubKey = newEntry.KeyInfo.PubKey
			srcInfo.TimeID_0 = newEntry.KeyInfo.TimeID_0
			srcInfo.TimeID_1 = newEntry.KeyInfo.TimeID_1
			srcEntry.PrivKey = nil
		}
	}

	return newTome, nil
}

// EqualTo compares if two key entries are identical/interchangable
func (entry *KeyEntry) EqualTo(other *KeyEntry) bool {
	a := entry.KeyInfo
	b := other.KeyInfo

	return a.KeyType == b.KeyType &&
		a.CryptoKitID == b.CryptoKitID &&
		a.TimeID_0 == b.TimeID_0 &&
		a.TimeID_1 == b.TimeID_1 &&
		bytes.Equal(a.PubKey, b.PubKey) &&
		bytes.Equal(entry.PrivKey, other.PrivKey)
}

// ZeroOut zeros out this entry's private key buffer
func (entry *KeyEntry) ZeroOut() {
	N := int32(len(entry.PrivKey))
	for i := int32(0); i < N; i++ {
		entry.PrivKey[i] = 0
	}
}

// Label returns a human readable label string for this KeyRef
func (kref *KeyRef) Label() string {
	return fmt.Sprintf("%s / %s", kref.KeyringID().Base32(), encode.ToBase32(kref.PubKey))
}

func (kref *KeyRef) KeyringID() tag.UID {
	return tag.UID{
		kref.KeyringID_0,
		kref.KeyringID_1,
	}
}

func (kref *KeyRef) SetKeyringID(uid tag.UID) {
	kref.KeyringID_0 = uid[0]
	kref.KeyringID_1 = uid[1]
}

// Zero overwrites a buffer with zeros.
func Zero(buf []byte) {
	for i := range buf {
		buf[i] = 0
	}
}

// GenerateNewKey creates a new key, blocking until completion.
// This is a convenience function wrapping Enclave.GenerateKeys for single-key generation.
func GenerateNewKey(
	enclave Enclave,
	keyringID tag.UID,
	keyInfo *KeyInfo,
) (*KeyInfo, error) {

	tomeOut, err := enclave.GenerateKeys(&KeyTome{
		Keyrings: []*Keyring{
			{
				UID_0: keyringID[0],
				UID_1: keyringID[1],
				Keys: []*KeyEntry{
					{
						KeyInfo: keyInfo,
					},
				},
			},
		},
	})

	if err != nil {
		return nil, err
	}

	var kr *Keyring
	if tomeOut != nil && len(tomeOut.Keyrings) > 0 {
		kr = tomeOut.Keyrings[0]
	}

	if kr == nil || len(kr.Keys) == 0 || kr.Keys[0] == nil || kr.Keys[0].KeyInfo == nil {
		return nil, status.Code_AssertFailed.Error("no keys returned")
	}

	if kr.Keys[0].KeyInfo.KeyType != keyInfo.KeyType {
		return nil, status.Code_AssertFailed.Error("unexpected key type")
	}

	if keyringID != kr.ID() {
		return nil, status.Code_AssertFailed.Error("generate returned different keyring name")
	}

	return kr.Keys[0].KeyInfo, nil
}

// FetchHasher returns the hash pkg for the given hash kit
func FetchHasher(hashKitID HashKitID) func() hash.Hash {
	var hasher func() hash.Hash

	switch hashKitID {

	case HashKitID_SHA3_256:
		hasher = sha3.New256

	case HashKitID_Blake2s_256:
		hasher = func() hash.Hash {
			inst, _ := blake2s.New256(nil)
			return inst
		}

	}

	return hasher
}

// NewHashKit returns the requested HashKit.
func NewHashKit(hashKitID HashKitID) (HashKit, error) {
	var kit HashKit

	if hashKitID == 0 {
		hashKitID = HashKitID_Blake2s_256
	}

	hasher := FetchHasher(hashKitID)
	if hasher == nil {
		return kit, status.Code_HashKitNotFound.Errorf("unrecognized HashKitID %v", hashKitID)
	}

	kit.HashKitID = hashKitID
	kit.Hasher = hasher()
	kit.HashSz = kit.Hasher.Size()

	return kit, nil
}
