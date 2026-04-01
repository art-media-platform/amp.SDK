package safe

import (
	"bytes"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"io"

	"github.com/art-media-platform/amp.SDK/stdlib/status"
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

// newAEAD creates an XChaCha20-Poly1305 AEAD from a 32-byte key.
func newAEAD(key []byte) (cipher interface {
	Seal(dst, nonce, plaintext, additionalData []byte) []byte
	Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error)
	NonceSize() int
}, err error) {
	if len(key) != DEKSize {
		return nil, fmt.Errorf("safe: key must be %d bytes, got %d", DEKSize, len(key))
	}
	return chacha20poly1305.NewX(key)
}

// deriveKey uses HKDF-SHA256 to derive a key from root material + salt + info.
func deriveKey(rootKey, salt, info []byte) ([]byte, error) {
	hk := hkdf.New(sha256.New, rootKey, salt, info)
	derived := make([]byte, DEKSize)
	if _, err := io.ReadFull(hk, derived); err != nil {
		return nil, fmt.Errorf("safe: HKDF derivation failed: %w", err)
	}
	return derived, nil
}

// sealAEAD encrypts plaintext using XChaCha20-Poly1305.
// Returns (nonce, ciphertext) where ciphertext includes the Poly1305 tag.
func sealAEAD(rand io.Reader, key, plaintext, aad []byte) (nonce, cipherblob []byte, err error) {
	aead, err := newAEAD(key)
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

// openAEAD decrypts ciphertext using XChaCha20-Poly1305.
func openAEAD(key, nonce, ciphertext, aad []byte) ([]byte, error) {
	aead, err := newAEAD(key)
	if err != nil {
		return nil, err
	}

	plaintext, err := aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("safe: AEAD decryption failed (bad key, corrupted data, or tampered AAD)")
	}
	return plaintext, nil
}

// generateDEK creates a fresh random DEK.
func generateDEK(rand io.Reader) ([]byte, error) {
	dek := make([]byte, DEKSize)
	if _, err := io.ReadFull(rand, dek); err != nil {
		return nil, fmt.Errorf("safe: failed to generate DEK: %w", err)
	}
	return dek, nil
}

/*****************************************************
** XChaCha20-Poly1305 CryptoKit (symmetric + X25519 asymmetric)
**/

func init() {
	RegisterCryptoKit(&xcKit)
}

var xcKit = CryptoKit{
	ID:          CryptoKitID_Poly25519,
	GenerateKey: xcGenerateKey,
	Encrypt:     xcEncrypt,
	Decrypt:     xcDecrypt,
	EncryptFor:  xcEncryptFor,
	DecryptFrom: xcDecryptFrom,
	Sign:        edSign,
	Verify:      edVerify,
}

// xcGenerateKey generates a new key pair based on KeyForm.
func xcGenerateKey(inRand io.Reader, inRequestedKeySz int, ioEntry *KeyEntry) error {
	keyInfo := ioEntry.KeyInfo

	switch keyInfo.KeyForm {
	case KeyForm_SymmetricKey:
		// PubKey acts as a public identifier; PrivKey is the symmetric secret.
		pubSz := inRequestedKeySz
		if pubSz < 16 {
			pubSz = 32
		}
		keyInfo.PubKey = make([]byte, pubSz)
		if _, err := io.ReadFull(inRand, keyInfo.PubKey); err != nil {
			return status.Code_KeyGenerationFailed.Wrap(err)
		}
		ioEntry.PrivKey = make([]byte, DEKSize)
		if _, err := io.ReadFull(inRand, ioEntry.PrivKey); err != nil {
			return status.Code_KeyGenerationFailed.Wrap(err)
		}

	case KeyForm_AsymmetricKey:
		curve := ecdh.X25519()
		priv, err := curve.GenerateKey(inRand)
		if err != nil {
			return status.Code_KeyGenerationFailed.Wrap(err)
		}
		keyInfo.PubKey = priv.PublicKey().Bytes()
		ioEntry.PrivKey = priv.Bytes()

	case KeyForm_SigningKey:
		pub, priv, err := ed25519.GenerateKey(inRand)
		if err != nil {
			return status.Code_KeyGenerationFailed.Wrap(err)
		}
		ioEntry.KeyInfo.PubKey = pub
		ioEntry.PrivKey = priv

	default:
		return status.ErrUnimplemented
	}
	return nil
}

// xcEncrypt encrypts inMsg with a symmetric key using XChaCha20-Poly1305.
// Output: nonce (24 bytes) || ciphertext+tag
func xcEncrypt(inRand io.Reader, inMsg []byte, inKey []byte) ([]byte, error) {
	if len(inKey) != DEKSize {
		return nil, status.Code_BadKeyFormat.Errorf("symmetric key must be %d bytes, got %d", DEKSize, len(inKey))
	}

	nonce, cipherblob, err := sealAEAD(inRand, inKey, inMsg, nil)
	if err != nil {
		return nil, err
	}
	return append(nonce, cipherblob...), nil
}

// xcDecrypt decrypts a buffer produced by xcEncrypt.
func xcDecrypt(inMsg []byte, inKey []byte) ([]byte, error) {
	if len(inKey) != DEKSize {
		return nil, status.Code_BadKeyFormat.Errorf("symmetric key must be %d bytes, got %d", DEKSize, len(inKey))
	}
	if len(inMsg) < NonceSize {
		return nil, status.Code_DecryptFailed.Error("ciphertext too short")
	}
	return openAEAD(inKey, inMsg[:NonceSize], inMsg[NonceSize:], nil)
}

// xcEncryptFor encrypts inMsg for a peer via X25519 ECDH + HKDF + XChaCha20-Poly1305.
// Output: nonce (24 bytes) || ciphertext+tag
func xcEncryptFor(inRand io.Reader, inMsg []byte, inPeerPubKey []byte, inPrivKey []byte) ([]byte, error) {
	sharedKey, err := x25519DeriveKey(inPrivKey, inPeerPubKey)
	if err != nil {
		return nil, err
	}
	defer Zero(sharedKey)
	return xcEncrypt(inRand, inMsg, sharedKey)
}

// xcDecryptFrom decrypts a buffer produced by xcEncryptFor.
func xcDecryptFrom(inMsg []byte, inPeerPubKey []byte, inPrivKey []byte) ([]byte, error) {
	sharedKey, err := x25519DeriveKey(inPrivKey, inPeerPubKey)
	if err != nil {
		return nil, err
	}
	defer Zero(sharedKey)
	return xcDecrypt(inMsg, sharedKey)
}

// x25519DeriveKey computes an ECDH shared secret and derives a symmetric key via HKDF.
func x25519DeriveKey(privKey []byte, peerPubKey []byte) ([]byte, error) {
	curve := ecdh.X25519()

	priv, err := curve.NewPrivateKey(privKey)
	if err != nil {
		return nil, status.Code_BadKeyFormat.Wrap(err)
	}
	peer, err := curve.NewPublicKey(peerPubKey)
	if err != nil {
		return nil, status.Code_BadKeyFormat.Wrap(err)
	}

	shared, err := priv.ECDH(peer)
	if err != nil {
		return nil, status.Code_DecryptFailed.Wrap(err)
	}
	defer Zero(shared)

	// Derive a symmetric key from the shared secret.
	// Using the concatenation of both public keys as HKDF info for domain separation.
	// Canonical order ensures both sides derive the same key.
	myPub := priv.PublicKey().Bytes()
	var lo, hi []byte
	if bytes.Compare(myPub, peerPubKey) <= 0 {
		lo, hi = myPub, peerPubKey
	} else {
		lo, hi = peerPubKey, myPub
	}
	info := append([]byte("safe.X25519."), lo...)
	info = append(info, hi...)

	return deriveKey(shared, nil, info)
}

func edSign(inDigest []byte, inSignerPrivKey []byte) ([]byte, error) {
	if len(inSignerPrivKey) != ed25519.PrivateKeySize {
		return nil, status.Code_BadKeyFormat.Errorf("bad ed25519 private key size: want %d, got %d", ed25519.PrivateKeySize, len(inSignerPrivKey))
	}
	sig := ed25519.Sign(inSignerPrivKey, inDigest)
	return sig, nil
}

func edVerify(inSig []byte, inDigest []byte, inSignerPubKey []byte) error {
	if len(inSignerPubKey) != ed25519.PublicKeySize {
		return status.Code_BadKeyFormat.Errorf("bad ed25519 public key size: want %d, got %d", ed25519.PublicKeySize, len(inSignerPubKey))
	}
	if !ed25519.Verify(inSignerPubKey, inDigest, inSig) {
		return status.Code_VerifySignatureFailed.Error("ed25519 signature verification failed")
	}
	return nil
}
