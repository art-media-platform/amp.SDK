// Package safe implements secure key storage and retrieval.
//
// Architecture:
//
//	Guard      — protects/recovers a DEK (Data Encryption Key) using root material.
//	             Implementations: fileGuard (local passphrase).
//
//	TomeStore  — persists a SealedTome to durable storage (file, cloud, etc).
//
//	Enclave    — the runtime session: loads a KeyTome via TomeStore+Guard,
//	             provides crypto ops, key management, and re-seals on Close().
//
//	Kit  — pluggable crypto implementation keyed by CryptoKitID.
//	             Nil function fields indicate unsupported capabilities.
package safe

import (
	"context"
	"crypto/rand"
	"io"

	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// RandReader is the default cryptographic random source.
// Tests may override this for deterministic output.
var RandReader io.Reader = rand.Reader

// ErrStoreClosed is the closed-keystore lifecycle sentinel: every EpochKeyStore
// method returns it once Close has sealed the session.  It is a typed,
// errors.Is-able identity so consumers can classify "the store is closed" apart
// from "the key is absent" or an AEAD refusal — a logout racing a decrypt must
// read as no-custody (retryable), never as forgery (final).  Any keystore a
// Guard source backs must honor the same contract.
var ErrStoreClosed = status.Code_Closed.Error("safe: key store is closed")

// Guard protects and recovers the DEK used to encrypt a KeyTome payload.
//
// Implementations:
//   - fileGuard  — derives a wrapping key from a passphrase
type Guard interface {

	// Info returns metadata about this Guard's capabilities.
	Info(ctx context.Context) (*GuardInfo, error)

	// WrapDEK protects a DEK under the Guard's root material.  The returned
	// WrappedDEK is self-describing and sufficient to recover the DEK.
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

// KeySpec describes a key to be generated.
type KeySpec struct {
	CryptoKitID   CryptoKitID
	KeyType       KeyType
	RequestedSize int // advisory; 0 = kit default
}

// Enclave is a live cryptographic session backed by an in-memory key index.
//
// On Open:  TomeStore.Load() -> Guard.UnwrapDEK() -> decrypt -> KeyTome -> index
// On Close: index -> KeyTome -> new DEK -> Guard.WrapDEK() -> encrypt -> TomeStore.Save()
//
// All methods are threadsafe.
type Enclave interface {

	// ImportKey inserts a keypair into the given keyring.
	// If kp.Pub.TimeID is zero, it is set to tag.NowID().
	// An exact-match duplicate is a no-op; a pub-key collision that is NOT an
	// exact dupe is rejected as an error.
	// The install is durable at method return: the re-sealed tome persists
	// BEFORE ImportKey reports success, so an unclean kill after return cannot
	// lose the key — a member's identity must never ride on a clean Close.
	// The guard must be open at install (it closes only at teardown, after all
	// installs); a persist failure returns the error and is NOT a durable install.
	ImportKey(ctx context.Context, keyringID tag.UID, kp KeyPair) error

	// GenerateKey creates a new keypair in the given keyring and registers it.
	// Returns the new PubKey (with TimeID populated).
	// Durable at return, like ImportKey.
	GenerateKey(ctx context.Context, keyringID tag.UID, spec KeySpec) (PubKey, error)

	// RemoveKey deletes the one keypair ref names exactly — keyring, Type and
	// the FULL PubKey; no newest-key or prefix resolution — and persists before
	// returning (durable like ImportKey); the private half is zeroed.  A ref
	// without Type or PubKey is BadRequest; a key not held is KeyringNotFound;
	// a persist failure leaves the key held and returns the error.  For a
	// caller withdrawing material it minted itself (a join that failed before
	// its record was written), never a resolver.
	RemoveKey(ctx context.Context, ref *KeyRef) error

	// FetchPubKey returns the PubKey for ref.
	// If len(ref.PubKey) == 0, the newest key in the keyring is returned.
	FetchPubKey(ref *KeyRef) (PubKey, error)

	// CanSign reports whether ref resolves to a SigningKey whose PRIVATE half is
	// held — i.e. this enclave can actually produce a signature for it.  A key
	// adopted public-only (a TOFU-declared peer key) has its pubkey but no PrvKey,
	// so FetchPubKey succeeds for it while CanSign returns false: "knows the
	// pubkey" is not "can sign."  Self-signing authorship must gate on CanSign.
	CanSign(ref *KeyRef) bool

	// SignRaw signs msg exactly as given — NO domain is applied.  Every call site
	// must pass a registry-derived digest (SigningDigest / CoSignatureDigest /
	// TxSignedDigest) or the documented MemberToken text-message exception; new
	// signing contexts go through SignDomain.
	SignRaw(ref *KeyRef, msg []byte) ([]byte, error)

	// EncryptSym encrypts plaintext using the SymmetricKey referenced by ref.
	// Output: nonce (24) || ciphertext+tag.
	EncryptSym(ref *KeyRef, plaintext []byte) ([]byte, error)

	// DecryptSym decrypts a buffer produced by EncryptSym using the same
	// SymmetricKey reference.
	DecryptSym(ref *KeyRef, ciphertext []byte) ([]byte, error)

	// OpenFromPub decrypts a sealed-box ciphertext (produced by safe.SealFor
	// or kit.Encrypt.Seal) using the AsymmetricKey referenced by ref.  The
	// ephemeral sender pubkey is parsed from the front of msg per the kit's
	// wire format.
	OpenFromPub(ref *KeyRef, msg []byte) ([]byte, error)

	// ExportSymmetricKey returns a copy of the raw symmetric key bytes for the
	// referenced keyring.  The caller is responsible for zeroing the returned
	// slice after use.
	//
	// This is intentionally limited to symmetric keys — signing and asymmetric
	// private keys MUST NOT leave the Enclave.  Symmetric epoch keys are exported
	// so that CryptoProvider can derive subkeys (content_key, proof_key) for
	// payload encryption and relay membership proofs.  The trust boundary is the
	// process, not the Enclave API.
	ExportSymmetricKey(ref *KeyRef) ([]byte, error)

	// Close re-seals the key index and persists it, then zeros sensitive material.
	Close(ctx context.Context) error
}

// KeyScope identifies a key's owning planet and optional channel. A channel
// remains distinct from planet scope even when their UIDs are equal.
type KeyScope struct {
	Kind      EpochKeyScope
	PlanetID  tag.UID
	ChannelID tag.UID
}

// Scope returns a planet key scope.
func Scope(planetID tag.UID) KeyScope {
	return KeyScope{
		Kind:     EpochKeyScope_ScopePlanet,
		PlanetID: planetID,
	}
}

// ChannelScope returns a channel key scope owned by planetID.
func ChannelScope(planetID, channelID tag.UID) KeyScope {
	return KeyScope{
		Kind:      EpochKeyScope_ScopeChannel,
		PlanetID:  planetID,
		ChannelID: channelID,
	}
}

// EpochKeyStore holds symmetric keys at (scope, epoch, role). All methods are
// threadsafe. Writes serialize across live stores sharing one tome identity.
// Pending destruction refuses writes to that scoped epoch with NotReady.
type EpochKeyStore interface {
	// PutKey explicitly replaces the addressed role. Publication is staged:
	// success means persisted; a Save error never publishes a new memory key.
	PutKey(ctx context.Context, scope KeyScope, key SymKey) error

	// InstallKey atomically installs an absent role, accepts identical held
	// material, and refuses conflicting bytes or crypto kit with AuthFailed.
	// An equal retry persists before success. Caller owns key.Bytes.
	InstallKey(ctx context.Context, scope KeyScope, key SymKey) (bool, error)

	// GetKey returns an owned copy; the caller must zero its Bytes.
	GetKey(scope KeyScope, epochID tag.UID, role KeyRole) (SymKey, error)
	GetCurrentKey(scope KeyScope, role KeyRole) (SymKey, error)
	SetCurrentEpoch(ctx context.Context, scope KeyScope, epochID tag.UID) error

	// ResolveChannelScope resolves one channel across ALL ownership claims,
	// including shredded entries and every role. Missing ownership is
	// KeyringNotFound; multiple channels or a planet-epoch alias is AuthFailed.
	ResolveChannelScope(planetID, epochID tag.UID) (KeyScope, error)

	// ShredKeys deletes every role in the exact scope, retaining non-secret
	// ownership. A shredded current has no implicit successor, including on
	// reopen. Save failures remain pending for retry/Close. Coordination is
	// process-local; this is not a cross-process custody invalidation protocol.
	ShredKeys(ctx context.Context, scope KeyScope, epochIDs []tag.UID) error
	Close(ctx context.Context) error
}

// CryptoKitID is a crypto kit's identity — the name-derived tag.UID of its Kit
// (see the Kit consts, e.g. safe.Crypto.Poly25519.ID).  It is an alias, so a
// CryptoKitID IS a tag.UID: the open suite namespace, carried on the wire as a
// fixed64 X_0/X_1 pair, and the key into the Kit registry.  Trust is pinned in
// signed EpochTerms — an unrecognized suite UID fails closed at verification.
// The nil UID means "unspecified" — resolve to the epoch default
// (EffectiveCryptoKit).
type CryptoKitID = tag.UID

/*****************************************************
** Kit — a Kit's pluggable implementation
**/

// Kit is the registered implementation of a crypto kit, identified by a
// CryptoKitID (see the Kit identity namespace).  It bundles two
// independent capability axes — signing and asymmetric encryption — so a kit
// can expose one, the other, or both.  Symmetric AEAD is kit-agnostic and lives
// on the safe package directly (SealAEAD / OpenAEAD).
//
// A hash is deliberately NOT a Kit axis: it has no keypair and no tie to the
// suite, so content-integrity hashing is its own orthogonal registry (see
// HashSpec / RegisterHashKit).
//
// Nil capability pointers mean "not supported by this kit" (e.g. a future
// Dilithium kit would expose Signing only; a Kyber kit would expose Encrypt
// only).
type Kit struct {
	ID      CryptoKitID
	Signing *SigningOps // identity / signatures; nil if kit doesn't sign
	Encrypt *EncryptOps // ECDH / asymmetric encrypt; nil if kit doesn't ECDH
}

// SigningOps bundles the signing-side primitives of a Kit.
// All non-nil functions must be threadsafe.
type SigningOps struct {
	// SignatureSize is the fixed byte length of signatures produced by Sign.
	SignatureSize int

	// Generate populates kp.Pub.Bytes and kp.Prv with a fresh SigningKey keypair
	// in this kit. The caller sets kp.Pub.CryptoKitID and kp.Pub.KeyType
	// beforehand.
	Generate func(rng io.Reader, kp *KeyPair) error

	// Sign produces a cryptographic signature of digest.
	Sign func(digest []byte, signerPrvKey []byte) ([]byte, error)

	// Verify validates a signature against a digest and public key.
	// Returns nil if the signature is valid.
	Verify func(sig []byte, digest []byte, signerPubKey []byte) error
}

// EncryptOps bundles the asymmetric-encryption primitives of a Kit.
// All non-nil functions must be threadsafe.
//
// The Seal/Open shape is an anonymous-sender sealed box:  Seal generates a
// fresh ephemeral keypair in this kit per
// call, performs ECDH against the recipient's pubkey, and embeds the ephemeral
// pubkey in the output.  Open recovers the ephemeral pubkey from the front of
// the ciphertext and ECDHs against the recipient's private key.  No sender
// identity participates in the wrap; sender authentication is the surrounding
// signed TxMsg's job.
//
// This shape eliminates the cross-kit ECDH constraint entirely: an admin in
// kit A can wrap to a recipient in kit B by generating an ephemeral in kit B.
// Admin doesn't need to maintain per-kit EncryptKeys; cross-kit rotation is
// asynchronous and member-driven (members rotate their EncryptKey on their
// own schedule; admin keeps wrapping in each member's current kit).
//
// Wire format (per kit):
//
//	eph_pub (kit-specific length) || nonce (24) || ciphertext+tag
type EncryptOps struct {
	// Generate populates kp.Pub.Bytes and kp.Prv with a fresh AsymmetricKey
	// keypair in this kit. The caller sets kp.Pub.CryptoKitID and kp.Pub.KeyType.
	Generate func(rng io.Reader, kp *KeyPair) error

	// Seal encrypts msg for a peer using an ephemeral sender keypair generated
	// in this kit.  No sender identity participates.
	Seal func(rng io.Reader, msg, peerPubKey []byte) ([]byte, error)

	// Open decrypts a buffer produced by Seal using only the recipient's
	// private key.  The ephemeral sender pubkey is parsed from the front of msg.
	Open func(msg, prvKey []byte) ([]byte, error)
}

// SealFor encrypts msg for a recipient's pubkey in the recipient's kit.  No
// Enclave required — the wrap is anonymous-sender (ephemeral keypair generated
// per call inside the kit).  Returns ciphertext containing the ephemeral pubkey
// embedded at the front per the kit's wire format.
//
// peerKit must match the kit that generated peerPubKey; the caller typically
// reads both from MemberEpoch.EncryptKey, a safe.KeyRef carrying kit and
// pubkey.
func SealFor(peerKit CryptoKitID, peerPubKey, msg []byte) ([]byte, error) {
	kit, err := CryptoKit(peerKit)
	if err != nil {
		return nil, err
	}
	if kit.Encrypt == nil || kit.Encrypt.Seal == nil {
		return nil, status.Code_Unsupported.Errorf("Kit %s does not support asymmetric encryption", peerKit.String())
	}
	return kit.Encrypt.Seal(RandReader, msg, peerPubKey)
}

// VerifySignature is a convenience function that performs signature validation
// for any registered Kit.  Returns nil if the signature is valid.
// This function is threadsafe.
func VerifySignature(
	cryptoKitID CryptoKitID,
	sig []byte,
	digest []byte,
	signerPubKey []byte,
) error {
	kit, err := CryptoKit(cryptoKitID)
	if err != nil {
		return err
	}
	if kit.Signing == nil || kit.Signing.Verify == nil {
		return status.Code_Unsupported.Errorf("Kit %s does not support signature verification", cryptoKitID.String())
	}
	return kit.Signing.Verify(sig, digest, signerPubKey)
}
