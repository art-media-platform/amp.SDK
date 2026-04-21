// Package p256 registers the P-256 CryptoKit with the safe package.
//
// This kit provides:
//   - Symmetric encryption: XChaCha20-Poly1305 (shared with Poly25519 for content interop)
//   - Asymmetric encryption: ECDH-P256 + HKDF + XChaCha20-Poly1305
//   - Signing: ECDSA-P256 over SHA-256
//
// A single 32-byte scalar serves as both the signing private key (ECDSA) and the
// key-agreement private key (ECDH). The 65-byte SEC1-uncompressed public key
// (0x04 || X || Y) is used for both operations, enabling one identity key to
// sign and to receive encrypted content — the same model YubiKey PIV expects.
//
// Import this package (typically via blank import) to register the kit:
//
//	import _ "github.com/art-media-platform/amp.SDK/stdlib/safe/p256"
package p256

import (
	"bytes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"io"
	"math/big"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"golang.org/x/crypto/chacha20poly1305"
)

const (
	// PrvKeySize is the raw big-endian scalar length.
	PrvKeySize = 32

	// PubKeySize is the SEC1 uncompressed public-key length: 0x04 || X || Y.
	PubKeySize = 65

	// SignatureSize is the raw r||s length (32 bytes each, big-endian).
	SignatureSize = 64
)

func init() {
	safe.RegisterCryptoKit(&kit)
}

var kit = safe.CryptoKit{
	ID:            safe.CryptoKitID_P256,
	SignatureSize: SignatureSize,
	GenerateKey:   generateKey,
	Encrypt:       encrypt,
	Decrypt:       decrypt,
	EncryptFor:    encryptFor,
	DecryptFrom:   decryptFrom,
	Sign:          sign,
	Verify:        verify,
	NewAEAD:       newAEAD,
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) != safe.DEKSize {
		return nil, status.Code_BadKeyFormat.Errorf("symmetric key must be %d bytes, got %d", safe.DEKSize, len(key))
	}
	return chacha20poly1305.NewX(key)
}

func generateKey(rng io.Reader, requestedSize int, kp *safe.KeyPair) error {
	switch kp.Pub.KeyType {
	case safe.KeyType_SymmetricKey:
		pubSize := requestedSize
		if pubSize < 16 {
			pubSize = 32
		}
		kp.Pub.Bytes = make([]byte, pubSize)
		if _, err := io.ReadFull(rng, kp.Pub.Bytes); err != nil {
			return status.Code_KeyGenerationFailed.Wrap(err)
		}
		kp.Prv = make([]byte, safe.DEKSize)
		if _, err := io.ReadFull(rng, kp.Prv); err != nil {
			return status.Code_KeyGenerationFailed.Wrap(err)
		}

	case safe.KeyType_SigningKey:
		priv, err := ecdh.P256().GenerateKey(rng)
		if err != nil {
			return status.Code_KeyGenerationFailed.Wrap(err)
		}
		kp.Prv = priv.Bytes()
		kp.Pub.Bytes = priv.PublicKey().Bytes()

	default:
		return status.ErrUnimplemented
	}
	return nil
}

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

func decrypt(msg []byte, key []byte) ([]byte, error) {
	if len(key) != safe.DEKSize {
		return nil, status.Code_BadKeyFormat.Errorf("symmetric key must be %d bytes, got %d", safe.DEKSize, len(key))
	}
	if len(msg) < safe.NonceSize {
		return nil, status.Code_DecryptFailed.Error("ciphertext too short")
	}
	return safe.OpenAEAD(key, msg[:safe.NonceSize], msg[safe.NonceSize:], nil)
}

func encryptFor(rng io.Reader, msg []byte, peerPubKey []byte, prvKey []byte) ([]byte, error) {
	shared, err := ecdhDeriveKey(prvKey, peerPubKey)
	if err != nil {
		return nil, err
	}
	defer safe.Zero(shared)
	return encrypt(rng, msg, shared)
}

func decryptFrom(msg []byte, peerPubKey []byte, prvKey []byte) ([]byte, error) {
	shared, err := ecdhDeriveKey(prvKey, peerPubKey)
	if err != nil {
		return nil, err
	}
	defer safe.Zero(shared)
	return decrypt(msg, shared)
}

// ecdhDeriveKey computes the ECDH shared secret and derives a symmetric key
// via HKDF with a canonical-ordered info string, matching the Poly25519 shape.
func ecdhDeriveKey(prvKey, peerPubKey []byte) ([]byte, error) {
	curve := ecdh.P256()

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

	myPub := prv.PublicKey().Bytes()
	var lo, hi []byte
	if bytes.Compare(myPub, peerPubKey) <= 0 {
		lo, hi = myPub, peerPubKey
	} else {
		lo, hi = peerPubKey, myPub
	}
	info := append([]byte("safe.ECDH-P256."), lo...)
	info = append(info, hi...)

	return safe.DeriveKey(shared, nil, info)
}

func sign(msg []byte, signerPrvKey []byte) ([]byte, error) {
	if len(signerPrvKey) != PrvKeySize {
		return nil, status.Code_BadKeyFormat.Errorf("P-256 private key must be %d bytes, got %d", PrvKeySize, len(signerPrvKey))
	}

	priv := &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{Curve: elliptic.P256()},
		D:         new(big.Int).SetBytes(signerPrvKey),
	}
	priv.PublicKey.X, priv.PublicKey.Y = elliptic.P256().ScalarBaseMult(signerPrvKey)

	digest := sha256.Sum256(msg)
	r, s, err := ecdsa.Sign(rand.Reader, priv, digest[:])
	if err != nil {
		return nil, status.Code_SigningFailed.Wrap(err)
	}

	sig := make([]byte, SignatureSize)
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	copy(sig[PrvKeySize-len(rBytes):], rBytes)
	copy(sig[SignatureSize-len(sBytes):], sBytes)
	return sig, nil
}

func verify(sig []byte, msg []byte, signerPubKey []byte) error {
	if len(sig) != SignatureSize {
		return status.Code_BadKeyFormat.Errorf("P-256 signature must be %d bytes, got %d", SignatureSize, len(sig))
	}
	if len(signerPubKey) != PubKeySize {
		return status.Code_BadKeyFormat.Errorf("P-256 public key must be %d bytes, got %d", PubKeySize, len(signerPubKey))
	}

	pubX, pubY := elliptic.Unmarshal(elliptic.P256(), signerPubKey)
	if pubX == nil {
		return status.Code_BadKeyFormat.Error("P-256 public key failed to unmarshal")
	}
	pub := &ecdsa.PublicKey{Curve: elliptic.P256(), X: pubX, Y: pubY}

	r := new(big.Int).SetBytes(sig[:PrvKeySize])
	s := new(big.Int).SetBytes(sig[PrvKeySize:])

	digest := sha256.Sum256(msg)
	if !ecdsa.Verify(pub, digest[:], r, s) {
		return status.Code_VerifySignatureFailed.Error("ECDSA-P256 signature verification failed")
	}
	return nil
}
