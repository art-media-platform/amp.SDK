package p256_test

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	_ "github.com/art-media-platform/amp.SDK/stdlib/safe/p256" // register P-256 KitSpec
)

// TestP256_KeyPair_Shape exercises key generation and confirms the on-wire
// byte sizes match the kit's declared constants (SEC1 uncompressed public,
// raw 32-byte scalar private).
func TestP256_KeyPair_Shape(t *testing.T) {
	kit, err := safe.GetKit(safe.CryptoKitID_P256)
	if err != nil {
		t.Fatalf("GetKit: %v", err)
	}

	kp := safe.KeyPair{
		Pub: safe.PubKey{CryptoKitID: safe.CryptoKitID_P256, KeyType: safe.KeyType_SigningKey},
	}
	if err := kit.Signing.Generate(rand.Reader, &kp); err != nil {
		t.Fatalf("Sign.Generate: %v", err)
	}

	if len(kp.Pub.Bytes) != 65 {
		t.Errorf("P-256 public key must be 65 bytes (SEC1 uncompressed), got %d", len(kp.Pub.Bytes))
	}
	if kp.Pub.Bytes[0] != 0x04 {
		t.Errorf("P-256 public key must lead with 0x04 (uncompressed tag), got 0x%02x", kp.Pub.Bytes[0])
	}
	if len(kp.Prv) != 32 {
		t.Errorf("P-256 private key must be 32 bytes (raw scalar), got %d", len(kp.Prv))
	}
	if kit.Signing.SignatureSize != 64 {
		t.Errorf("P-256 signature size must be 64 bytes (r||s), got %d", kit.Signing.SignatureSize)
	}
}

// TestP256_SignVerify exercises the primary identity-key use case: sign a
// canonical byte blob (e.g. a PlanetEpoch) and verify with the public key.
// This is the shape `PlanetEpoch.VerifyCoSignature` drives.
func TestP256_SignVerify(t *testing.T) {
	kit, err := safe.GetKit(safe.CryptoKitID_P256)
	if err != nil {
		t.Fatalf("GetKit: %v", err)
	}

	kp := safe.KeyPair{
		Pub: safe.PubKey{CryptoKitID: safe.CryptoKitID_P256, KeyType: safe.KeyType_SigningKey},
	}
	if err := kit.Signing.Generate(rand.Reader, &kp); err != nil {
		t.Fatalf("Sign.Generate: %v", err)
	}

	msg := []byte("genesis PlanetEpoch canonical bytes")
	sig, err := kit.Signing.Sign(msg, kp.Prv)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if len(sig) != kit.Signing.SignatureSize {
		t.Fatalf("signature size mismatch: got %d, want %d", len(sig), kit.Signing.SignatureSize)
	}

	if err := kit.Signing.Verify(sig, msg, kp.Pub.Bytes); err != nil {
		t.Fatalf("Verify should succeed: %v", err)
	}

	// Tampered message must fail.  P-256 ECDSA signatures are deterministic over
	// the message digest (SHA-256), so any single-bit change in the input produces
	// a digest the signature no longer validates against.  An attacker cannot
	// re-use a valid signature for a different message.
	bad := append([]byte{}, msg...)
	bad[0] ^= 0xFF
	if err := kit.Signing.Verify(sig, bad, kp.Pub.Bytes); err == nil {
		t.Fatal("Verify must reject tampered message")
	}

	// Tampered signature must fail.
	tamperedSig := append([]byte{}, sig...)
	tamperedSig[0] ^= 0xFF
	if err := kit.Signing.Verify(tamperedSig, msg, kp.Pub.Bytes); err == nil {
		t.Fatal("Verify must reject tampered signature")
	}
}

// TestP256_SymmetricRoundtrip exercises the shared-key AEAD path.  Symmetric
// AEAD is kit-agnostic across all registered kits — XChaCha20-Poly1305 — so an
// epoch key wrapped by one kit is readable by any other; this is what enables
// mixed-suite quorums.
func TestP256_SymmetricRoundtrip(t *testing.T) {
	key := make([]byte, safe.DEKSize)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}

	plaintext := []byte("epoch-key-wrapped symmetric content")
	nonce, ct, err := safe.SealAEAD(rand.Reader, key, plaintext, nil)
	if err != nil {
		t.Fatalf("SealAEAD: %v", err)
	}
	ciphertext := append(nonce, ct...)
	if bytes.Equal(plaintext, ciphertext) {
		t.Fatal("ciphertext must differ from plaintext")
	}

	got, err := safe.OpenAEAD(key, ciphertext[:safe.NonceSize], ciphertext[safe.NonceSize:], nil)
	if err != nil {
		t.Fatalf("OpenAEAD: %v", err)
	}
	if !bytes.Equal(plaintext, got) {
		t.Fatalf("symmetric roundtrip mismatch: got %q, want %q", got, plaintext)
	}
}

// TestP256_SealedBox exercises the anonymous-sender wrap (RFC 9180 base mode):
// Seal generates an ephemeral keypair, ECDHs against the recipient's pubkey,
// and embeds the ephemeral pubkey in the ciphertext.  Open uses only the
// recipient's prv key.  This is how epoch keys are sealed to a new member at
// invite time and during rotation.
func TestP256_SealedBox(t *testing.T) {
	kit, err := safe.GetKit(safe.CryptoKitID_P256)
	if err != nil {
		t.Fatalf("GetKit: %v", err)
	}

	bob := freshKeyPair(t, kit)

	secret := []byte("planet epoch key material")
	sealed, err := kit.Encrypt.Seal(rand.Reader, secret, bob.Pub.Bytes)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	opened, err := kit.Encrypt.Open(sealed, bob.Prv)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !bytes.Equal(secret, opened) {
		t.Fatalf("sealed-box roundtrip mismatch: got %q, want %q", opened, secret)
	}

	eve := freshKeyPair(t, kit)
	if _, err := kit.Encrypt.Open(sealed, eve.Prv); err == nil {
		t.Fatal("Open must fail with non-recipient private key")
	}
}

// TestP256_Registered confirms the kit self-registers via import side-effect,
// so callers that do `import _ ".../p256"` can `safe.GetKit(P256)`.
func TestP256_Registered(t *testing.T) {
	kit, err := safe.GetKit(safe.CryptoKitID_P256)
	if err != nil {
		t.Fatalf("P-256 kit must be registered via blank import: %v", err)
	}
	if kit.ID != safe.CryptoKitID_P256 {
		t.Errorf("registered kit has wrong ID: got %v, want %v", kit.ID, safe.CryptoKitID_P256)
	}
	if kit.Signing == nil || kit.Encrypt == nil {
		t.Errorf("P-256 kit must expose both Sign and Encr capabilities")
	}
}

func freshKeyPair(t *testing.T, kit *safe.KitSpec) safe.KeyPair {
	t.Helper()
	kp := safe.KeyPair{
		Pub: safe.PubKey{CryptoKitID: safe.CryptoKitID_P256, KeyType: safe.KeyType_AsymmetricKey},
	}
	if err := kit.Encrypt.Generate(rand.Reader, &kp); err != nil {
		t.Fatal(err)
	}
	return kp
}
