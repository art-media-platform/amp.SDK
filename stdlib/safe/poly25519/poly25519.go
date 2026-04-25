// Package poly25519 registers the Poly25519 CryptoKit with the safe package.
//
// This kit provides:
//   - Asymmetric encryption (KeyType_AsymmetricKey): X25519 ECDH + HKDF + XChaCha20-Poly1305
//   - Signing (KeyType_SigningKey): Ed25519
//
// SigningKeys and AsymmetricKeys are independent keypairs.  Symmetric AEAD lives at
// the safe package level (safe.SealAEAD / safe.OpenAEAD) and is kit-agnostic.
//
// Import this package (typically via blank import) to register the kit:
//
//	import _ "github.com/art-media-platform/amp.SDK/stdlib/safe/poly25519"
package poly25519

import (
	"bytes"
	"crypto/ecdh"
	"crypto/ed25519"
	"io"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/status"
)

func init() {
	safe.RegisterKit(&kit)
}

var kit = safe.KitSpec{
	ID: safe.CryptoKitID_Poly25519,
	Signing: &safe.SigningOps{
		SignatureSize: ed25519.SignatureSize,
		Generate:      generateSignKey,
		Sign:          sign,
		Verify:        verify,
	},
	Encrypt: &safe.EncryptOps{
		Generate: generateEncKey,
		Seal:     seal,
		Open:     open,
	},
}

// generateSignKey produces a fresh Ed25519 keypair.
func generateSignKey(rng io.Reader, kp *safe.KeyPair) error {
	pub, priv, err := ed25519.GenerateKey(rng)
	if err != nil {
		return status.Code_KeyGenerationFailed.Wrap(err)
	}
	kp.Pub.Bytes = pub
	kp.Prv = priv
	return nil
}

// generateEncKey produces a fresh X25519 keypair (independent of the sign key).
func generateEncKey(rng io.Reader, kp *safe.KeyPair) error {
	priv, err := ecdh.X25519().GenerateKey(rng)
	if err != nil {
		return status.Code_KeyGenerationFailed.Wrap(err)
	}
	kp.Prv = priv.Bytes()
	kp.Pub.Bytes = priv.PublicKey().Bytes()
	return nil
}

// seal encrypts msg for a peer via X25519 ECDH + HKDF + XChaCha20-Poly1305.
// Both prvKey and peerPubKey must be 32-byte X25519 keys.
// Output: nonce (24 bytes) || ciphertext+tag
func seal(rng io.Reader, msg []byte, peerPubKey []byte, prvKey []byte) ([]byte, error) {
	sharedKey, err := x25519DeriveKey(prvKey, peerPubKey)
	if err != nil {
		return nil, err
	}
	defer safe.Zero(sharedKey)
	nonce, ct, err := safe.SealAEAD(rng, sharedKey, msg, nil)
	if err != nil {
		return nil, err
	}
	return append(nonce, ct...), nil
}

// open decrypts a buffer produced by seal.
func open(msg []byte, peerPubKey []byte, prvKey []byte) ([]byte, error) {
	sharedKey, err := x25519DeriveKey(prvKey, peerPubKey)
	if err != nil {
		return nil, err
	}
	defer safe.Zero(sharedKey)
	if len(msg) < safe.NonceSize {
		return nil, status.Code_DecryptFailed.Error("ciphertext too short")
	}
	return safe.OpenAEAD(sharedKey, msg[:safe.NonceSize], msg[safe.NonceSize:], nil)
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
