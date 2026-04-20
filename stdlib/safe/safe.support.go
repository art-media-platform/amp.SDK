package safe

import (
	"fmt"
	"hash"

	"github.com/art-media-platform/amp.SDK/stdlib/encode"
	"github.com/art-media-platform/amp.SDK/stdlib/status"
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

// FetchHasher returns the hash pkg for the given hash kit.
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
