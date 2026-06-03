package safe_test

import (
	"crypto/rand"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"

	_ "github.com/art-media-platform/amp.SDK/stdlib/safe/poly25519" // register the Poly25519 suite
)

// TestCryptoKit_DowngradeFailsClosed proves that an open suite namespace is NOT
// open trust.  A valid Poly25519 signature verifies under the Poly25519 suite UID,
// but the SAME signature presented under any other suite UID the verifier does not
// hold is rejected: the registry fails closed with Code_ItemNotFound rather than
// silently accepting or panicking.
//
// This is the substrate guarantee behind epoch trust-pinning: a member's key
// declares its suite by UID, verification resolves that UID against the registered
// set, and a tx that names an unrecognized/untrusted suite can never be downgraded
// into acceptance — it fails closed before any signature math runs.
func TestCryptoKit_DowngradeFailsClosed(t *testing.T) {
	kit, err := safe.CryptoKit(safe.Crypto.Poly25519.ID)
	if err != nil {
		t.Fatalf("Poly25519 suite must be registered: %v", err)
	}

	kp := safe.KeyPair{
		Pub: safe.PubKey{CryptoKitID: safe.Crypto.Poly25519.ID, KeyType: safe.KeyType_SigningKey},
	}
	if err := kit.Signing.Generate(rand.Reader, &kp); err != nil {
		t.Fatalf("generate signing key: %v", err)
	}

	digest := make([]byte, 32)
	if _, err := rand.Read(digest); err != nil {
		t.Fatal(err)
	}
	sig, err := kit.Signing.Sign(digest, kp.Prv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	// Honest path: the suite the key actually declares verifies.
	if err := safe.VerifySignature(safe.Crypto.Poly25519.ID, sig, digest, kp.Pub.Bytes); err != nil {
		t.Fatalf("honest verify under Poly25519 must pass: %v", err)
	}

	// Downgrade attempt: the identical signature presented under a different,
	// unregistered suite UID must fail closed (Code_ItemNotFound).
	bogus := tag.NameFrom("net.attacker.downgrade").ID
	if err := safe.VerifySignature(bogus, sig, digest, kp.Pub.Bytes); status.GetCode(err) != status.Code_ItemNotFound {
		t.Fatalf("downgrade under an unregistered suite must fail closed (ItemNotFound); got %v", err)
	}
}
