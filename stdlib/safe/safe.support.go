package safe

import (
	"crypto/sha256"
	"fmt"
	"hash"

	"github.com/art-media-platform/amp.SDK/stdlib/encode"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"golang.org/x/crypto/blake2s"
	"golang.org/x/crypto/sha3"
)

const (
	// MinPubKeyPrefixSz prevents suspiciously small pub key prefixes from being used.
	MinPubKeyPrefixSz = 16
)

// HashKit is an abstraction for hash.Hash.
type HashKit struct {
	HashKitID HashKitID
	Hasher    hash.Hash
	HashSz    int
}

// ContainerID returns the ContainerID as a tag.UID.
func (ek *EpochKeyEntry) ContainerID() tag.UID {
	return tag.UID{ek.ContainerID_0, ek.ContainerID_1}
}

// EpochID returns the EpochID as a tag.UID.
func (ek *EpochKeyEntry) EpochID() tag.UID {
	return tag.UID{ek.EpochID_0, ek.EpochID_1}
}

// Label returns a human readable label string for this KeyRef.
func (kref *KeyRef) Label() string {
	return fmt.Sprintf("%s / %s", kref.KeyringID().Base32(), encode.ToBase32(kref.PubKey))
}

// KeyringID returns the KeyringID as a tag.UID.
func (kref *KeyRef) KeyringID() tag.UID {
	return tag.UID{
		kref.KeyringID_0,
		kref.KeyringID_1,
	}
}

// SetKeyringID sets the KeyringID from a tag.UID.
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

// HashSpec is the registration recipe for a content-integrity hash: its ID, a
// factory for a fresh stateful hasher, and the digest size.  NewHashKit resolves
// a live HashKit from it.
//
// Hashing is an axis orthogonal to the CryptoKit suite — a hash has no keypair
// and no tie to the curve — so it lives in its own registry (gHashKits), never
// as a field on CryptoKit.
type HashSpec struct {
	ID   HashKitID
	New  func() hash.Hash
	Size int
}

// NewHashKit returns a live HashKit (a fresh hasher) for the requested kit.
// The zero value resolves to the default content hash (Blake2s_256).
func NewHashKit(hashKitID HashKitID) (HashKit, error) {
	var kit HashKit

	spec, err := GetHashKit(hashKitID)
	if err != nil {
		return kit, err
	}

	kit.HashKitID = spec.ID
	kit.Hasher = spec.New()
	kit.HashSz = spec.Size

	return kit, nil
}

// The dependency-light content hashes register from this package's init().
// BLAKE3 pulls a SIMD dependency and so registers from amp.planet (blank import).
func init() {
	mustRegisterHashKit(&HashSpec{
		ID:   HashKitID_Blake2s_256,
		New:  func() hash.Hash { h, _ := blake2s.New256(nil); return h },
		Size: 32,
	})
	mustRegisterHashKit(&HashSpec{
		ID:   HashKitID_SHA2_256,
		New:  sha256.New,
		Size: 32,
	})
	mustRegisterHashKit(&HashSpec{
		ID:   HashKitID_SHA3_256,
		New:  sha3.New256,
		Size: 32,
	})
}

// mustRegisterHashKit panics if registration fails — an ID collision in init() is
// a build-time programming error, not a runtime condition.
func mustRegisterHashKit(spec *HashSpec) {
	if err := RegisterHashKit(spec); err != nil {
		panic(err)
	}
}
