package safe

import (
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// KeyType_NumTypes bounds the per-keyring per-type indexing arrays.
// Must be greater than the largest KeyType enum value.
const KeyType_NumTypes = 4

// PubKey is a runtime descriptor for a public key and the CryptoKit that produced it.
// Bytes is the raw public key material (signing pub key; CryptoKit derives asymmetric
// keys from it on demand).
type PubKey struct {
	CryptoKitID CryptoKitID
	KeyType     KeyType
	TimeID      tag.UID
	Bytes       []byte
}

// Clone returns an independent copy of pk.
func (pk PubKey) Clone() PubKey {
	out := pk
	if pk.Bytes != nil {
		out.Bytes = append([]byte(nil), pk.Bytes...)
	}
	return out
}

// KeyPair is a runtime asymmetric key pair. Prv holds private key material and
// must be zeroed (via Zero) when no longer needed.
type KeyPair struct {
	Pub PubKey
	Prv []byte
}

// Zero overwrites the private key bytes.
func (kp *KeyPair) Zero() {
	Zero(kp.Prv)
	kp.Prv = nil
}

// SymKey is a runtime symmetric key, carrying its CryptoKit, EpochID, and Role.
// A given epoch may carry up to 4 distinct SymKeys (one per KeyRole) — access-tiered
// distribution places different roles in different members' hands (see KeyRole).
// Bytes must be zeroed (via Zero) when no longer needed.
type SymKey struct {
	CryptoKitID CryptoKitID
	EpochID     tag.UID
	Role        KeyRole
	Bytes       []byte
}

// IsSet reports whether the key carries usable material.
func (sk SymKey) IsSet() bool {
	return len(sk.Bytes) > 0
}

// Clone returns an independent copy of sk. Caller owns the returned Bytes.
func (sk SymKey) Clone() SymKey {
	out := sk
	if sk.Bytes != nil {
		out.Bytes = append([]byte(nil), sk.Bytes...)
	}
	return out
}

// Zero overwrites the key bytes.
func (sk *SymKey) Zero() {
	Zero(sk.Bytes)
	sk.Bytes = nil
}
