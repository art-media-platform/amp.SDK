// Package safe implements secure key storage and retrieval.
//
// Architecture:
//
//	Guard      — protects/recovers a DEK (Data Encryption Key) using root material.
//	             Implementations: fileGuard (local passphrase), yubiGuard (YubiKey PIV).
//
//	TomeStore  — persists a SealedTome to durable storage (file, cloud, etc).
//
//	Enclave    — the runtime session: loads a KeyTome via TomeStore+Guard,
//	             provides crypto ops, key management, and re-seals on Close().
//
//	CryptoKit  — pluggable crypto implementation keyed by CryptoKitID.
//	             Nil function fields indicate unsupported capabilities.
package safe

import (
	"context"
	"crypto/rand"
	"io"
	"sync"

	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// RandReader is the default cryptographic random source.
// Tests may override this for deterministic output.
var RandReader io.Reader = rand.Reader

// Guard protects and recovers the DEK used to encrypt a KeyTome payload.
//
// Implementations:
//   - fileGuard  — derives a wrapping key from a passphrase via HKDF
//   - yubiGuard  — uses YubiKey PIV key agreement to derive a wrapping key
type Guard interface {

	// Info returns metadata about this Guard's capabilities.
	Info(ctx context.Context) (*GuardInfo, error)

	// WrapDEK protects a DEK under the Guard's root material.
	// The returned WrappedDEK is self-describing and sufficient to recover the DEK.
	WrapDEK(ctx context.Context, dek []byte, aad []byte) (*WrappedDEK, error)

	// UnwrapDEK recovers the original DEK from a WrappedDEK.
	UnwrapDEK(ctx context.Context, wrapped *WrappedDEK, aad []byte) (dek []byte, err error)

	// Close releases any resources held by this Guard.
	Close() error
}

// TomeStore persists and retrieves a SealedTome.
//
// Implementations:
//   - localTomeStore — reads/writes a single file on the local filesystem
type TomeStore interface {
	Load(ctx context.Context) (*SealedTome, error)
	Save(ctx context.Context, sealed *SealedTome) error
}

// Enclave is a live cryptographic session backed by an in-memory KeyTome.
//
// On Open:  TomeStore.Load() -> Guard.UnwrapDEK() -> decrypt -> KeyTome
// On Close: new DEK -> Guard.WrapDEK() -> encrypt(KeyTome) -> TomeStore.Save()
//
// The KeyTome is fully internal — callers interact through the methods below.
// All methods are threadsafe.
type Enclave interface {

	// Merges all keys in the given KeyTome with this host KeyTome.
	// See docs for KeyTome.MergeTome() on how error conditions are addressed.
	// Note: incoming duplicate key entries are ignored/dropped.
	ImportKeys(srcTome *KeyTome) error

	// Generates a new KeyEntry for each entry in srcTome (based on the entry's KeyType and CryptoKitID, ignoring the rest)
	// and merges it with the host KeyTome.  A copy of each newly generated entry (except for PrivKey) is placed into the result KeyTome.
	// See "KeyGen mode" notes where KeyEntry is declared.
	GenerateKeys(srcTome *KeyTome) (*KeyTome, error)

	// Returns info about a key for the referenced key.
	// If len(inKeyRef.PubKey) == 0, then the newest KeyEntry in the implied Keyring is returned.
	FetchKeyInfo(inKeyRef *KeyRef) (*KeyInfo, error)

	// Performs signing, encryption, and decryption.
	DoCryptOp(inArgs *CryptOpArgs) (*CryptOpOut, error)

	// ExportSymmetricKey returns a copy of the raw symmetric key bytes for the referenced keyring.
	// The caller is responsible for zeroing the returned slice after use.
	//
	// This is intentionally limited to symmetric keys — signing and asymmetric private keys
	// MUST NOT leave the Enclave.  Symmetric epoch keys are exported so that CryptoProvider
	// can derive subkeys (content_key, proof_key) via HKDF for payload encryption and
	// relay membership proofs.  The trust boundary is the process, not the Enclave API.
	ExportSymmetricKey(inKeyRef *KeyRef) ([]byte, error)

	// Close re-seals the KeyTome and persists it, then zeros sensitive material.
	Close(ctx context.Context) error
}

// EpochKeyStore manages symmetric epoch keys separately from identity keys.
//
// Symmetric epoch keys have fundamentally different access patterns from identity keys:
//   - High volume: up to millions of keys per user across all planets/channels
//   - Must be exported for HKDF derivation (content_key, proof_key)
//   - Hot/cold separation: only current epoch keys need to be in memory
//   - Simple put/get by (containerID, epochID) — no PubKey indexing needed
//
// All methods are threadsafe.
type EpochKeyStore interface {

	// PutKey stores a symmetric epoch key for the given container (planet or channel).
	PutKey(containerID, epochID tag.UID, cryptoKit CryptoKitID, key []byte) error

	// GetKey retrieves a symmetric epoch key by its container and epoch UIDs.
	// Returns a copy of the key bytes and its CryptoKitID; the caller must zero the key after use.
	GetKey(containerID, epochID tag.UID) ([]byte, CryptoKitID, error)

	// GetCurrentKey returns the current (most recent) epoch key for a container.
	// Returns the epochID and a copy of the key bytes.
	GetCurrentKey(containerID tag.UID) (epochID tag.UID, key []byte, err error)

	// SetCurrentEpoch marks an epoch as the current one for a container.
	SetCurrentEpoch(containerID, epochID tag.UID) error

	// Close encrypts and persists all keys, then zeros sensitive material.
	Close(ctx context.Context) error
}

/*****************************************************
** CryptoKit — pluggable crypto implementation
**/

// CryptoKit is a pluggable cryptographic suite identified by a CryptoKitID.
// Each function field implements a specific capability; nil fields mean "not supported."
// All non-nil functions must be threadsafe.
type CryptoKit struct {
	ID CryptoKitID

	// SignatureSize is the fixed byte length of signatures produced by this kit's Sign function.
	SignatureSize int

	// GenerateKey populates ioEntry.KeyInfo.PubKey and ioEntry.PrivKey based on ioEntry.KeyInfo.KeyType.
	// Pre: ioEntry.KeyInfo.KeyType and .CryptoKitID are set; TimeID is set by GenerateFork/caller.
	// inRequestedKeySz is advisory (ignored by some implementations).
	GenerateKey func(inRand io.Reader, inRequestedKeySz int, ioEntry *KeyEntry) error

	// Encrypt encrypts inMsg with a symmetric key.
	Encrypt func(inRand io.Reader, inMsg []byte, inKey []byte) ([]byte, error)

	// Decrypt decrypts a buffer produced by Encrypt.
	Decrypt func(inMsg []byte, inKey []byte) ([]byte, error)

	// EncryptFor encrypts inMsg for a peer using asymmetric key agreement.
	// The kit derives asymmetric keys from signing keys if needed (implementation-specific).
	EncryptFor func(inRand io.Reader, inMsg []byte, inPeerPubKey []byte, inPrivKey []byte) ([]byte, error)

	// DecryptFrom decrypts a buffer produced by EncryptFor.
	// The kit derives asymmetric keys from signing keys if needed (implementation-specific).
	DecryptFrom func(inMsg []byte, inPeerPubKey []byte, inPrivKey []byte) ([]byte, error)

	// Sign produces a cryptographic signature of inDigest.
	Sign func(inDigest []byte, inSignerPrivKey []byte) ([]byte, error)

	// Verify validates a signature against a digest and public key.
	// Returns nil if the signature is valid.
	Verify func(inSig []byte, inDigest []byte, inSignerPubKey []byte) error
}

/*****************************************************
** CryptoKit registry
**/

// gRegistry maps a CryptoKitID to an available ("registered") CryptoKit.
var gRegistry struct {
	sync.RWMutex
	Lookup map[CryptoKitID]*CryptoKit
}

// RegisterCryptoKit registers the given CryptoKit so it can be retrieved via GetCryptoKit().
// It is safe to call from init().  Registering the same kit twice (same pointer) is a no-op.
func RegisterCryptoKit(kit *CryptoKit) error {
	var err error
	gRegistry.Lock()
	if gRegistry.Lookup == nil {
		gRegistry.Lookup = map[CryptoKitID]*CryptoKit{}
	}
	existing := gRegistry.Lookup[kit.ID]
	if existing == nil {
		gRegistry.Lookup[kit.ID] = kit
	} else if existing != kit {
		err = status.Code_UnrecognizedCryptoKit.Errorf("CryptoKit %d (%s) is already registered", kit.ID, kit.ID.String())
	}
	gRegistry.Unlock()
	return err
}

// GetCryptoKit fetches a registered CryptoKit by its ID.
// If the associated CryptoKit has not been registered, an error is returned.
func GetCryptoKit(cryptoKitID CryptoKitID) (*CryptoKit, error) {
	gRegistry.RLock()
	kit := gRegistry.Lookup[cryptoKitID]
	gRegistry.RUnlock()

	if kit == nil {
		return nil, status.Code_CryptoKitAlreadyRegistered.Errorf("CryptoKit %d not found", cryptoKitID)
	}
	return kit, nil
}

// VerifySignature is a convenience function that performs signature validation for any registered CryptoKit.
// Returns nil if the signature of inDigest plus the signer's private key matches the given signature.
// This function is threadsafe.
func VerifySignature(
	cryptoKitID CryptoKitID,
	sig []byte,
	digest []byte,
	signerPubKey []byte,
) error {
	kit, err := GetCryptoKit(cryptoKitID)
	if err != nil {
		return err
	}
	if kit.Verify == nil {
		return status.Code_Unimplemented.Errorf("CryptoKit %d does not support signature verification", cryptoKitID)
	}
	return kit.Verify(sig, digest, signerPubKey)
}
