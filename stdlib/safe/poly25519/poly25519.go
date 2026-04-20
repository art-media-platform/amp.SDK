// Package poly25519 registers the Poly25519 CryptoKit with the safe package.
//
// This kit provides:
//   - Symmetric encryption: XChaCha20-Poly1305
//   - Asymmetric encryption: X25519 ECDH key agreement + HKDF + XChaCha20-Poly1305
//   - Signing: Ed25519
//
// Signing keys (Ed25519) serve as the single identity key. X25519 asymmetric keys
// are derived at runtime via the Edwards-to-Montgomery birational map (public)
// and SHA-512 + RFC 7748 clamping (private). This derivation is entirely internal
// to this kit, preserving CryptoKit compartmentalization.
//
// Import this package (typically via blank import) to register the kit:
//
//	import _ "github.com/art-media-platform/amp.SDK/stdlib/safe/poly25519"
package poly25519

import (
	"bytes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/sha512"
	"io"

	"filippo.io/edwards25519"
	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"golang.org/x/crypto/chacha20poly1305"
)

func init() {
	safe.RegisterCryptoKit(&kit)
}

var kit = safe.CryptoKit{
	ID:            safe.CryptoKitID_Poly25519,
	SignatureSize: ed25519.SignatureSize,
	GenerateKey:   generateKey,
	Encrypt:       encrypt,
	Decrypt:       decrypt,
	EncryptFor:    encryptFor,
	DecryptFrom:   decryptFrom,
	Sign:          sign,
	Verify:        verify,
	NewAEAD:       newAEAD,
}

// newAEAD returns an XChaCha20-Poly1305 AEAD for streaming seal/open with
// per-chunk nonces and AAD. Used by blob stream crypto.
func newAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != safe.DEKSize {
		return nil, status.Code_BadKeyFormat.Errorf("symmetric key must be %d bytes, got %d", safe.DEKSize, len(key))
	}
	return chacha20poly1305.NewX(key)
}

// resolveAsymKeys converts signing keys to asymmetric keys for ECDH when needed.
// If prvKey is a signing key (64 bytes for this kit), both keys are converted;
// otherwise they are used as-is (already in asymmetric form).
func resolveAsymKeys(prvKey, peerPubKey []byte) (prvAsym, peerAsym []byte, err error) {
	if len(prvKey) == ed25519.PrivateKeySize {
		prvAsym, err = deriveAsymPrv(prvKey)
		if err != nil {
			return nil, nil, err
		}
		peerAsym, err = deriveAsymPub(peerPubKey)
		if err != nil {
			safe.Zero(prvAsym)
			return nil, nil, err
		}
		return prvAsym, peerAsym, nil
	}
	return prvKey, peerPubKey, nil
}

// deriveAsymPub converts an Ed25519 public key to X25519 via the Edwards->Montgomery birational map.
func deriveAsymPub(edPub []byte) ([]byte, error) {
	if len(edPub) != ed25519.PublicKeySize {
		return nil, status.Code_BadKeyFormat.Errorf("signing public key must be %d bytes, got %d", ed25519.PublicKeySize, len(edPub))
	}
	pt, err := new(edwards25519.Point).SetBytes(edPub)
	if err != nil {
		return nil, status.Code_BadKeyFormat.Errorf("invalid signing public key: %v", err)
	}
	return pt.BytesMontgomery(), nil
}

// deriveAsymPrv derives an X25519 private key from an Ed25519 private key.
func deriveAsymPrv(edPrv []byte) ([]byte, error) {
	if len(edPrv) != ed25519.PrivateKeySize {
		return nil, status.Code_BadKeyFormat.Errorf("signing private key must be %d bytes, got %d", ed25519.PrivateKeySize, len(edPrv))
	}
	hash := sha512.Sum512(edPrv[:ed25519.SeedSize])
	hash[0] &= 248
	hash[31] &= 127
	hash[31] |= 64
	out := make([]byte, 32)
	copy(out, hash[:32])
	return out, nil
}

// generateKey generates a new key pair based on KeyType.
func generateKey(rng io.Reader, requestedSize int, kp *safe.KeyPair) error {
	switch kp.Pub.KeyType {
	case safe.KeyType_SymmetricKey:
		// PubKey acts as a public identifier; PrivKey is the symmetric secret.
		pubSz := requestedSize
		if pubSz < 16 {
			pubSz = 32
		}
		kp.Pub.Bytes = make([]byte, pubSz)
		if _, err := io.ReadFull(rng, kp.Pub.Bytes); err != nil {
			return status.Code_KeyGenerationFailed.Wrap(err)
		}
		kp.Prv = make([]byte, safe.DEKSize)
		if _, err := io.ReadFull(rng, kp.Prv); err != nil {
			return status.Code_KeyGenerationFailed.Wrap(err)
		}

	case safe.KeyType_SigningKey:
		pub, priv, err := ed25519.GenerateKey(rng)
		if err != nil {
			return status.Code_KeyGenerationFailed.Wrap(err)
		}
		kp.Pub.Bytes = pub
		kp.Prv = priv

	default:
		return status.ErrUnimplemented
	}
	return nil
}

// encrypt encrypts msg with a symmetric key using XChaCha20-Poly1305.
// Output: nonce (24 bytes) || ciphertext+tag
func encrypt(rng io.Reader, msg []byte, key []byte) ([]byte, error) {
	if len(key) != safe.DEKSize {
		return nil, status.Code_BadKeyFormat.Errorf("symmetric key must be %d bytes, got %d", safe.DEKSize, len(key))
	}

	nonce, cipherblob, err := safe.SealAEAD(rng, key, msg, nil)
	if err != nil {
		return nil, err
	}
	return append(nonce, cipherblob...), nil
}

// decrypt decrypts a buffer produced by encrypt.
func decrypt(msg []byte, key []byte) ([]byte, error) {
	if len(key) != safe.DEKSize {
		return nil, status.Code_BadKeyFormat.Errorf("symmetric key must be %d bytes, got %d", safe.DEKSize, len(key))
	}
	if len(msg) < safe.NonceSize {
		return nil, status.Code_DecryptFailed.Error("ciphertext too short")
	}
	return safe.OpenAEAD(key, msg[:safe.NonceSize], msg[safe.NonceSize:], nil)
}

// encryptFor encrypts msg for a peer via ECDH key agreement + HKDF + XChaCha20-Poly1305.
// Signing keys are automatically converted to asymmetric keys for ECDH.
// Output: nonce (24 bytes) || ciphertext+tag
func encryptFor(rng io.Reader, msg []byte, peerPubKey []byte, prvKey []byte) ([]byte, error) {
	prvX, peerX, err := resolveAsymKeys(prvKey, peerPubKey)
	if err != nil {
		return nil, err
	}
	if prvX != nil {
		defer safe.Zero(prvX)
	}
	sharedKey, err := x25519DeriveKey(prvX, peerX)
	if err != nil {
		return nil, err
	}
	defer safe.Zero(sharedKey)
	return encrypt(rng, msg, sharedKey)
}

// decryptFrom decrypts a buffer produced by encryptFor.
// Signing keys are automatically converted to asymmetric keys for ECDH.
func decryptFrom(msg []byte, peerPubKey []byte, prvKey []byte) ([]byte, error) {
	prvX, peerX, err := resolveAsymKeys(prvKey, peerPubKey)
	if err != nil {
		return nil, err
	}
	if prvX != nil {
		defer safe.Zero(prvX)
	}
	sharedKey, err := x25519DeriveKey(prvX, peerX)
	if err != nil {
		return nil, err
	}
	defer safe.Zero(sharedKey)
	return decrypt(msg, sharedKey)
}

// x25519DeriveKey computes an ECDH shared secret and derives a symmetric key via HKDF.
func x25519DeriveKey(prvKey []byte, peerPubKey []byte) ([]byte, error) {
	curve := ecdh.X25519()

	prv, err := curve.NewPrivateKey(prvKey)
	if err != nil {
		return nil, status.Code_BadKeyFormat.Wrap(err)
	}
	peer, err := curve.NewPublicKey(peerPubKey)
	if err != nil {
		return nil, status.Code_BadKeyFormat.Wrap(err)
	}

	shared, err := prv.ECDH(peer)
	if err != nil {
		return nil, status.Code_DecryptFailed.Wrap(err)
	}
	defer safe.Zero(shared)

	// Derive a symmetric key from the shared secret.
	// Using the concatenation of both public keys as HKDF info for domain separation.
	// Canonical order ensures both sides derive the same key.
	myPub := prv.PublicKey().Bytes()
	var lo, hi []byte
	if bytes.Compare(myPub, peerPubKey) <= 0 {
		lo, hi = myPub, peerPubKey
	} else {
		lo, hi = peerPubKey, myPub
	}
	info := append([]byte("safe.X25519."), lo...)
	info = append(info, hi...)

	return safe.DeriveKey(shared, nil, info)
}

func sign(digest []byte, signerPrvKey []byte) ([]byte, error) {
	if len(signerPrvKey) != ed25519.PrivateKeySize {
		return nil, status.Code_BadKeyFormat.Errorf("bad ed25519 private key size: want %d, got %d", ed25519.PrivateKeySize, len(signerPrvKey))
	}
	sig := ed25519.Sign(signerPrvKey, digest)
	return sig, nil
}

func verify(sig []byte, digest []byte, signerPubKey []byte) error {
	if len(signerPubKey) != ed25519.PublicKeySize {
		return status.Code_BadKeyFormat.Errorf("bad ed25519 public key size: want %d, got %d", ed25519.PublicKeySize, len(signerPubKey))
	}
	if !ed25519.Verify(signerPubKey, digest, sig) {
		return status.Code_VerifySignatureFailed.Error("ed25519 signature verification failed")
	}
	return nil
}
