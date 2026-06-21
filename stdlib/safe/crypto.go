package safe

import (
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

const (
	// DEKSize is the standard Data Encryption Key size (256 bits).
	DEKSize = chacha20poly1305.KeySize // 32

	// CipherName is the AEAD cipher used for all seal/unseal operations.
	CipherName = "XChaCha20-Poly1305"

	// KDFName identifies the KDF used to derive wrapping keys.
	KDFName = "HKDF-SHA256"

	// NonceSize is the XChaCha20-Poly1305 nonce size (192 bits).
	NonceSize = chacha20poly1305.NonceSizeX // 24

	// SaltSize for HKDF derivation.
	SaltSize = 32
)

/*****************************************************
** Generic AEAD + HKDF primitives
**
** These are used by CryptoKit implementations and by the Enclave/Guard
** infrastructure for at-rest encryption. CryptoKit authors may use these
** to build their kit without duplicating low-level cipher operations.
**/

// NewAEAD creates an AEAD cipher from a 32-byte key using the default cipher suite.
// Legacy callers that don't know the CryptoKitID (Enclave, Guard, SealAEAD) use this.
// Cipher-agnostic callers should use NewAEADForKit(kitID, key) instead.
func NewAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != DEKSize {
		return nil, fmt.Errorf("safe: key must be %d bytes, got %d", DEKSize, len(key))
	}
	return chacha20poly1305.NewX(key)
}

// NewAEADForKit returns a streaming AEAD for callers that received (key,
// cryptoKitID) from an EpochKeyStore or similar.  All registered kits use
// XChaCha20-Poly1305 for symmetric AEAD, so this just routes to NewAEAD; the
// kitID parameter is retained to satisfy callers that pass it for completeness.
func NewAEADForKit(cryptoKitID CryptoKitID, key []byte) (cipher.AEAD, error) {
	_ = cryptoKitID
	return NewAEAD(key)
}

// DeriveKey uses HKDF-SHA256 to derive a key from root material + salt + info.
func DeriveKey(rootKey, salt, info []byte) ([]byte, error) {
	hk := hkdf.New(sha256.New, rootKey, salt, info)
	derived := make([]byte, DEKSize)
	if _, err := io.ReadFull(hk, derived); err != nil {
		return nil, fmt.Errorf("safe: HKDF derivation failed: %w", err)
	}
	return derived, nil
}

// DeriveSubKey derives a purpose-specific subkey from a master key using HKDF-SHA256.
// Purpose strings provide domain separation (e.g. "member-proof", "content").
func DeriveSubKey(masterKey []byte, purpose string) ([]byte, error) {
	return DeriveKey(masterKey, nil, []byte(purpose))
}

// StretchParams names the tunable work factors for StretchKey.  They travel with
// the derived key (in a credential record or a sealed token) so a derivation can
// be upgraded without breaking older material.
type StretchParams struct {
	MemoryKiB uint32 // memory cost (KiB)
	Time      uint32 // iterations
	Threads   uint8  // parallelism
	KeyLen    uint32 // output length (bytes)
}

// DefaultStretchParams is the OWASP-2023 floor: 19 MiB, time=2, parallelism=1,
// 32-byte output — slow enough to make offline guessing expensive, fast enough
// that an interactive derive feels instant.
var DefaultStretchParams = StretchParams{
	MemoryKiB: 19 * 1024,
	Time:      2,
	Threads:   1,
	KeyLen:    DEKSize,
}

// StretchKey turns a low-entropy secret (a passphrase) into a key using a
// memory-hard, work-factored KDF, for inputs that must resist offline brute
// force.  Use DeriveKey (HKDF) for high-entropy root keys, where no work factor
// is needed.  (Implemented with Argon2id.)
func StretchKey(secret, salt []byte, params StretchParams) []byte {
	return argon2.IDKey(secret, salt, params.Time, params.MemoryKiB, params.Threads, params.KeyLen)
}

// ComputeHMAC computes HMAC-SHA256 over msg using the given key.
func ComputeHMAC(key, msg []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(msg)
	return mac.Sum(nil)
}

// VerifyHMAC checks that tag is a valid HMAC-SHA256 of msg under key.
func VerifyHMAC(key, msg, tag []byte) bool {
	expected := ComputeHMAC(key, msg)
	return hmac.Equal(expected, tag)
}

// SealAEAD encrypts plaintext using the AEAD cipher.
// Returns (nonce, ciphertext) where ciphertext includes the authentication tag.
func SealAEAD(rand io.Reader, key, plaintext, aad []byte) (nonce, cipherblob []byte, err error) {
	aead, err := NewAEAD(key)
	if err != nil {
		return nil, nil, err
	}

	nonce = make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand, nonce); err != nil {
		return nil, nil, fmt.Errorf("safe: failed to generate nonce: %w", err)
	}

	cipherblob = aead.Seal(nil, nonce, plaintext, aad)
	return nonce, cipherblob, nil
}

// OpenAEAD decrypts ciphertext using the AEAD cipher.
func OpenAEAD(key, nonce, ciphertext, aad []byte) ([]byte, error) {
	aead, err := NewAEAD(key)
	if err != nil {
		return nil, err
	}

	plaintext, err := aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("safe: AEAD decryption failed (bad key, corrupted data, or tampered AAD)")
	}
	return plaintext, nil
}

// GenerateDEK creates a fresh random DEK.
func GenerateDEK(rand io.Reader) ([]byte, error) {
	dek := make([]byte, DEKSize)
	if _, err := io.ReadFull(rand, dek); err != nil {
		return nil, fmt.Errorf("safe: failed to generate DEK: %w", err)
	}
	return dek, nil
}
