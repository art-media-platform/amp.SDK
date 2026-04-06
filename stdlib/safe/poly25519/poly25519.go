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
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/sha512"
	"io"

	"filippo.io/edwards25519"
	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/status"
)

func init() {
	safe.RegisterCryptoKit(&kit)
}

var kit = safe.CryptoKit{
	ID:          safe.CryptoKitID_Poly25519,
	GenerateKey: generateKey,
	Encrypt:     encrypt,
	Decrypt:     decrypt,
	EncryptFor:  encryptFor,
	DecryptFrom: decryptFrom,
	Sign:        sign,
	Verify:      verify,
}

// resolveAsymKeys converts signing keys to asymmetric keys for ECDH when needed.
// If privKey is a signing key (64 bytes for this kit), both keys are converted;
// otherwise they are used as-is (already in asymmetric form).
func resolveAsymKeys(privKey, peerPubKey []byte) (privAsym, peerAsym []byte, err error) {
	if len(privKey) == ed25519.PrivateKeySize {
		privAsym, err = deriveAsymPriv(privKey)
		if err != nil {
			return nil, nil, err
		}
		peerAsym, err = deriveAsymPub(peerPubKey)
		if err != nil {
			safe.Zero(privAsym)
			return nil, nil, err
		}
		return privAsym, peerAsym, nil
	}
	return privKey, peerPubKey, nil
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

// deriveAsymPriv derives an X25519 private key from an Ed25519 private key.
func deriveAsymPriv(edPriv []byte) ([]byte, error) {
	if len(edPriv) != ed25519.PrivateKeySize {
		return nil, status.Code_BadKeyFormat.Errorf("signing private key must be %d bytes, got %d", ed25519.PrivateKeySize, len(edPriv))
	}
	hash := sha512.Sum512(edPriv[:ed25519.SeedSize])
	hash[0] &= 248
	hash[31] &= 127
	hash[31] |= 64
	out := make([]byte, 32)
	copy(out, hash[:32])
	return out, nil
}

// generateKey generates a new key pair based on KeyType.
func generateKey(inRand io.Reader, inRequestedKeySz int, ioEntry *safe.KeyEntry) error {
	keyInfo := ioEntry.KeyInfo

	switch keyInfo.KeyType {
	case safe.KeyType_SymmetricKey:
		// PubKey acts as a public identifier; PrivKey is the symmetric secret.
		pubSz := inRequestedKeySz
		if pubSz < 16 {
			pubSz = 32
		}
		keyInfo.PubKey = make([]byte, pubSz)
		if _, err := io.ReadFull(inRand, keyInfo.PubKey); err != nil {
			return status.Code_KeyGenerationFailed.Wrap(err)
		}
		ioEntry.PrivKey = make([]byte, safe.DEKSize)
		if _, err := io.ReadFull(inRand, ioEntry.PrivKey); err != nil {
			return status.Code_KeyGenerationFailed.Wrap(err)
		}

	case safe.KeyType_SigningKey:
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

// encrypt encrypts inMsg with a symmetric key using XChaCha20-Poly1305.
// Output: nonce (24 bytes) || ciphertext+tag
func encrypt(inRand io.Reader, inMsg []byte, inKey []byte) ([]byte, error) {
	if len(inKey) != safe.DEKSize {
		return nil, status.Code_BadKeyFormat.Errorf("symmetric key must be %d bytes, got %d", safe.DEKSize, len(inKey))
	}

	nonce, cipherblob, err := safe.SealAEAD(inRand, inKey, inMsg, nil)
	if err != nil {
		return nil, err
	}
	return append(nonce, cipherblob...), nil
}

// decrypt decrypts a buffer produced by encrypt.
func decrypt(inMsg []byte, inKey []byte) ([]byte, error) {
	if len(inKey) != safe.DEKSize {
		return nil, status.Code_BadKeyFormat.Errorf("symmetric key must be %d bytes, got %d", safe.DEKSize, len(inKey))
	}
	if len(inMsg) < safe.NonceSize {
		return nil, status.Code_DecryptFailed.Error("ciphertext too short")
	}
	return safe.OpenAEAD(inKey, inMsg[:safe.NonceSize], inMsg[safe.NonceSize:], nil)
}

// encryptFor encrypts inMsg for a peer via ECDH key agreement + HKDF + XChaCha20-Poly1305.
// Signing keys are automatically converted to asymmetric keys for ECDH.
// Output: nonce (24 bytes) || ciphertext+tag
func encryptFor(inRand io.Reader, inMsg []byte, inPeerPubKey []byte, inPrivKey []byte) ([]byte, error) {
	privX, peerX, err := resolveAsymKeys(inPrivKey, inPeerPubKey)
	if err != nil {
		return nil, err
	}
	if privX != nil {
		defer safe.Zero(privX)
	}
	sharedKey, err := x25519DeriveKey(privX, peerX)
	if err != nil {
		return nil, err
	}
	defer safe.Zero(sharedKey)
	return encrypt(inRand, inMsg, sharedKey)
}

// decryptFrom decrypts a buffer produced by encryptFor.
// Signing keys are automatically converted to asymmetric keys for ECDH.
func decryptFrom(inMsg []byte, inPeerPubKey []byte, inPrivKey []byte) ([]byte, error) {
	privX, peerX, err := resolveAsymKeys(inPrivKey, inPeerPubKey)
	if err != nil {
		return nil, err
	}
	if privX != nil {
		defer safe.Zero(privX)
	}
	sharedKey, err := x25519DeriveKey(privX, peerX)
	if err != nil {
		return nil, err
	}
	defer safe.Zero(sharedKey)
	return decrypt(inMsg, sharedKey)
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
	defer safe.Zero(shared)

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

	return safe.DeriveKey(shared, nil, info)
}

func sign(inDigest []byte, inSignerPrivKey []byte) ([]byte, error) {
	if len(inSignerPrivKey) != ed25519.PrivateKeySize {
		return nil, status.Code_BadKeyFormat.Errorf("bad ed25519 private key size: want %d, got %d", ed25519.PrivateKeySize, len(inSignerPrivKey))
	}
	sig := ed25519.Sign(inSignerPrivKey, inDigest)
	return sig, nil
}

func verify(inSig []byte, inDigest []byte, inSignerPubKey []byte) error {
	if len(inSignerPubKey) != ed25519.PublicKeySize {
		return status.Code_BadKeyFormat.Errorf("bad ed25519 public key size: want %d, got %d", ed25519.PublicKeySize, len(inSignerPubKey))
	}
	if !ed25519.Verify(inSignerPubKey, inDigest, inSig) {
		return status.Code_VerifySignatureFailed.Error("ed25519 signature verification failed")
	}
	return nil
}
