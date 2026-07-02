package safe_test

import (
	"bytes"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	_ "github.com/art-media-platform/amp.SDK/stdlib/safe/poly25519"
)

// TestSigningDigest_DomainSeparation is the core property: the same payload under
// two different domains yields two different digests, so a signature made in one
// context can never verify in another.
func TestSigningDigest_DomainSeparation(t *testing.T) {
	payload := []byte("the same 32ish bytes signed in two contexts")

	loginD, err := safe.SigningDigest(0, safe.SigningDomain_Login, payload)
	if err != nil {
		t.Fatal(err)
	}
	txD, err := safe.SigningDigest(0, safe.SigningDomain_TxAuthor, payload)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(loginD, txD) {
		t.Fatal("distinct domains must produce distinct digests over the same payload")
	}
	if len(loginD) != 32 {
		t.Fatalf("digest length %d, want 32", len(loginD))
	}

	// Determinism: same inputs → same digest.
	again, _ := safe.SigningDigest(0, safe.SigningDomain_Login, payload)
	if !bytes.Equal(loginD, again) {
		t.Fatal("SigningDigest must be deterministic")
	}
}

// TestSigningDigest_TagIsLengthPrefixed guards against a domain that is a prefix
// of another colliding via a crafted payload — the length prefix makes the
// tag/payload boundary unambiguous.
func TestSigningDigest_TagIsLengthPrefixed(t *testing.T) {
	// Two distinct domains where one is a prefix of the other, distinguished only
	// by the length byte the tag carries.
	a := safe.SigningDomainTag(safe.SigningDomain("amp.sig"))
	b := safe.SigningDomainTag(safe.SigningDomain("amp.sig.tx.v1"))
	if bytes.HasPrefix(b, a) {
		t.Fatal("length-prefixed tags must not be prefixes of one another")
	}
	if int(a[0]) != len("amp.sig") {
		t.Fatalf("tag length prefix %d, want %d", a[0], len("amp.sig"))
	}
}

// TestSignVerifyDomain_RoundTripAndCrossReject: a domain signature verifies in
// its own context and is REJECTED when checked under any other domain.
func TestSignVerifyDomain_RoundTripAndCrossReject(t *testing.T) {
	kit, err := safe.CryptoKit(safe.Crypto.Poly25519.ID)
	if err != nil {
		t.Fatal(err)
	}
	kp := safe.KeyPair{Pub: safe.PubKey{CryptoKitID: safe.Crypto.Poly25519.ID, KeyType: safe.KeyType_SigningKey}}
	if err := kit.Signing.Generate(safe.RandReader, &kp); err != nil {
		t.Fatal(err)
	}
	payload := []byte("federation founding facts")

	// Sign directly with the kit over the login-domain digest (no enclave needed).
	digest, err := safe.SigningDigest(0, safe.SigningDomain_Login, payload)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := kit.Signing.Sign(digest, kp.Prv)
	if err != nil {
		t.Fatal(err)
	}

	// Verifies in its own domain.
	if err := safe.VerifyDomain(safe.Crypto.Poly25519.ID, 0, safe.SigningDomain_Login, sig, kp.Pub.Bytes, payload); err != nil {
		t.Fatalf("same-domain verify must succeed: %v", err)
	}
	// Rejected under every other domain — the whole point.
	for _, other := range safe.AllSigningDomains {
		if other == safe.SigningDomain_Login {
			continue
		}
		if err := safe.VerifyDomain(safe.Crypto.Poly25519.ID, 0, other, sig, kp.Pub.Bytes, payload); err == nil {
			t.Fatalf("a %s signature must NOT verify under %s", safe.SigningDomain_Login, other)
		}
	}
}

// TestSigningDomains_Distinct asserts the registry has no accidental duplicates —
// two contexts sharing a domain string would silently re-enable cross-reuse.
func TestSigningDomains_Distinct(t *testing.T) {
	all := safe.AllSigningDomains
	seen := make(map[safe.SigningDomain]bool, len(all))
	for _, d := range all {
		if d == "" {
			t.Fatal("a signing domain must not be empty")
		}
		if seen[d] {
			t.Fatalf("duplicate signing domain: %q", d)
		}
		seen[d] = true
	}
}

// TestSigningDomains_LengthBounded guards the u8 length prefix in SigningDomainTag:
// a 256-plus-byte domain would truncate mod-256 and could alias a shorter domain's
// tag, re-opening cross-domain collision.  The registry is closed and short today;
// this fails loudly the moment a future domain crosses the boundary.
func TestSigningDomains_LengthBounded(t *testing.T) {
	for _, d := range safe.AllSigningDomains {
		if len(d) > 255 {
			t.Fatalf("signing domain %q is %d bytes; the u8 length prefix truncates above 255", d, len(d))
		}
	}
}
