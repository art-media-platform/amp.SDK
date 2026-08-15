package amp_test

import (
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	_ "github.com/art-media-platform/amp.SDK/stdlib/safe/poly25519" // register the Poly25519 kit the fixtures sign with
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// newSigner mints a fresh Poly25519 signing keypair with an identity UID.
func newSigner(t *testing.T) (tag.UID, safe.PubKey, []byte) {
	t.Helper()
	kitID := safe.Crypto.Poly25519.ID
	kit, err := safe.CryptoKit(kitID)
	if err != nil {
		t.Fatal(err)
	}
	kp := safe.KeyPair{Pub: safe.PubKey{CryptoKitID: kitID, KeyType: safe.KeyType_SigningKey}}
	if err := kit.Signing.Generate(rand.Reader, &kp); err != nil {
		t.Fatal(err)
	}
	return tag.NewID(), kp.Pub, kp.Prv
}

func signDigest(t *testing.T, digest, prv []byte) []byte {
	t.Helper()
	kit, err := safe.CryptoKit(safe.Crypto.Poly25519.ID)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := kit.Signing.Sign(digest, prv)
	if err != nil {
		t.Fatal(err)
	}
	return sig
}

func signingKeyRef(pub []byte) *safe.KeyRef {
	ref := &safe.KeyRef{Type: safe.KeyType_SigningKey, PubKey: pub}
	ref.SetKit(safe.Crypto.Poly25519.ID)
	return ref
}

// reKeyFixture builds a well-formed re-key MemberEpoch for memberID on
// planetID, co-signed over its digest by each (signerID → private key).
func reKeyFixture(t *testing.T, planetID, memberID tag.UID, signers map[tag.UID][]byte) *amp.MemberEpoch {
	t.Helper()
	record := &amp.MemberEpoch{
		MemberTag:  amp.TagFromUID(memberID),
		Status:     amp.MemberStatus_Active,
		ReKeyPrior: signingKeyRef([]byte("prior-signing-key-32-bytes-....32")[:32]),
		SigningKey: signingKeyRef([]byte("new-signing-key-32-bytes-......32")[:32]),
		EncryptKey: signingKeyRef([]byte("new-encrypt-key-32-bytes-......32")[:32]),
	}
	digest, err := record.ReKeyCoSignatureDigest(planetID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for signerID, prv := range signers {
		record.ReKey = append(record.ReKey, &amp.CoSignature{
			MemberTag: amp.TagFromUID(signerID),
			Signature: signDigest(t, digest, prv),
		})
	}
	return record
}

// TestVerifyCoSignatureQuorum pins the frame-agnostic core's threshold
// semantics: distinct-signer counting, required <= 0 = all, and the
// never-fail-open floor.
func TestVerifyCoSignatureQuorum(t *testing.T) {
	digest := []byte("a-32-byte-digest-stand-in.......")
	id1, pub1, prv1 := newSigner(t)
	id2, pub2, prv2 := newSigner(t)
	id3, pub3, _ := newSigner(t)
	keys := map[tag.UID]safe.PubKey{id1: pub1, id2: pub2, id3: pub3}

	cosig := func(id tag.UID, prv []byte) *amp.CoSignature {
		return &amp.CoSignature{MemberTag: amp.TagFromUID(id), Signature: signDigest(t, digest, prv)}
	}
	two := []*amp.CoSignature{cosig(id1, prv1), cosig(id2, prv2)}

	if _, err := amp.VerifyCoSignatureQuorum(two, digest, keys, 2); err != nil {
		t.Errorf("2 valid signatures must satisfy required=2: %v", err)
	}
	if _, err := amp.VerifyCoSignatureQuorum(two, digest, keys, 0); err == nil {
		t.Error("required=0 means ALL declared signers; 2 of 3 must fail")
	}
	// A signer cannot manufacture a quorum by signing repeatedly.
	dup := []*amp.CoSignature{cosig(id1, prv1), cosig(id1, prv1)}
	if _, err := amp.VerifyCoSignatureQuorum(dup, digest, keys, 2); err == nil {
		t.Error("duplicate signer must count once — required=2 unmet")
	}
	// A signature naming no declared signer fails the whole quorum.
	strangerID, _, strangerPrv := newSigner(t)
	stranger := []*amp.CoSignature{cosig(id1, prv1), cosig(strangerID, strangerPrv)}
	if _, err := amp.VerifyCoSignatureQuorum(stranger, digest, keys, 1); err == nil {
		t.Error("a CoSignature from an undeclared signer must fail")
	}
	// Never fail-open: an empty signer set is unauthorized even with required<=0.
	if _, err := amp.VerifyCoSignatureQuorum(nil, digest, map[tag.UID]safe.PubKey{}, 0); err == nil {
		t.Error("empty signer set must never verify")
	}
	// A wrong-digest signature fails.
	if _, err := amp.VerifyCoSignatureQuorum(two, []byte("some-other-digest-32-bytes......"), keys, 1); err == nil {
		t.Error("a signature over a different digest must fail")
	}
}

// TestReKeyDigest_BindsFrame pins that every half of the re-key frame binds:
// planet, member, prior key, new signing key, new encrypt key — and that a
// record missing any key half refuses to produce a digest (fail-closed).
func TestReKeyDigest_BindsFrame(t *testing.T) {
	planetID, memberID := tag.NewID(), tag.NewID()
	base := reKeyFixture(t, planetID, memberID, nil)
	baseline, err := base.ReKeyCoSignatureDigest(planetID, 0)
	if err != nil {
		t.Fatal(err)
	}

	if other, _ := base.ReKeyCoSignatureDigest(tag.NewID(), 0); string(other) == string(baseline) {
		t.Error("digest must bind the planet")
	}
	mutant := reKeyFixture(t, planetID, tag.NewID(), nil)
	if other, _ := mutant.ReKeyCoSignatureDigest(planetID, 0); string(other) == string(baseline) {
		t.Error("digest must bind the member")
	}
	for _, swap := range []struct {
		name   string
		mutate func(me *amp.MemberEpoch)
	}{
		{"prior key", func(me *amp.MemberEpoch) { me.ReKeyPrior = signingKeyRef([]byte("a-different-prior-key-32-bytes.32")[:32]) }},
		{"new signing key", func(me *amp.MemberEpoch) { me.SigningKey = signingKeyRef([]byte("a-different-signing-key-32-byte32")[:32]) }},
		{"new encrypt key", func(me *amp.MemberEpoch) { me.EncryptKey = signingKeyRef([]byte("a-different-encrypt-key-32-byte32")[:32]) }},
	} {
		mutant := reKeyFixture(t, planetID, memberID, nil)
		swap.mutate(mutant)
		if other, _ := mutant.ReKeyCoSignatureDigest(planetID, 0); string(other) == string(baseline) {
			t.Errorf("digest must bind the %s", swap.name)
		}
	}
	for _, missing := range []struct {
		name   string
		mutate func(me *amp.MemberEpoch)
	}{
		{"prior key", func(me *amp.MemberEpoch) { me.ReKeyPrior = nil }},
		{"new signing key", func(me *amp.MemberEpoch) { me.SigningKey = nil }},
		{"new encrypt key", func(me *amp.MemberEpoch) { me.EncryptKey = nil }},
	} {
		mutant := reKeyFixture(t, planetID, memberID, nil)
		missing.mutate(mutant)
		if _, err := mutant.ReKeyCoSignatureDigest(planetID, 0); err == nil {
			t.Errorf("a re-key record missing its %s must refuse a digest", missing.name)
		}
	}
}

// TestVerifyReKeyQuorum pins the one re-key verify site: quorum semantics over
// the digest, the Status=Active reinstate requirement (Status rides outside
// the signed digest and must not be a submitter smuggling lane), and the
// structural did:key refusal.
func TestVerifyReKeyQuorum(t *testing.T) {
	planetID, memberID := tag.NewID(), tag.NewID()
	id1, pub1, prv1 := newSigner(t)
	id2, pub2, prv2 := newSigner(t)
	founders := map[tag.UID]safe.PubKey{id1: pub1, id2: pub2}
	signers := map[tag.UID][]byte{id1: prv1, id2: prv2}

	good := reKeyFixture(t, planetID, memberID, signers)
	if _, err := good.VerifyReKeyQuorum(planetID, 0, founders, 0); err != nil {
		t.Errorf("a fully co-signed re-key must verify: %v", err)
	}
	// Transplant refusal: the same signatures die under another planet.
	if _, err := good.VerifyReKeyQuorum(tag.NewID(), 0, founders, 0); err == nil {
		t.Error("a re-key blob transplanted to another planet must fail")
	}
	// Tampered new key after signing → digest mismatch.
	tampered := reKeyFixture(t, planetID, memberID, signers)
	tampered.SigningKey = signingKeyRef([]byte("swapped-in-attacker-key-32-byte32")[:32])
	if _, err := tampered.VerifyReKeyQuorum(planetID, 0, founders, 0); err == nil {
		t.Error("a post-signing key swap must fail the quorum")
	}
	// Partial quorum with required=0 (all founders) fails.
	partial := reKeyFixture(t, planetID, memberID, map[tag.UID][]byte{id1: prv1})
	if _, err := partial.VerifyReKeyQuorum(planetID, 0, founders, 0); err == nil {
		t.Error("required=0 demands all founders; one of two must fail")
	}
	if _, err := partial.VerifyReKeyQuorum(planetID, 0, founders, 1); err != nil {
		t.Errorf("one of two with required=1 must verify: %v", err)
	}
	// A re-key record must reinstate Status = Active.
	suspended := reKeyFixture(t, planetID, memberID, signers)
	suspended.Status = amp.MemberStatus_Suspended
	if _, err := suspended.VerifyReKeyQuorum(planetID, 0, founders, 0); err == nil {
		t.Error("a re-key record not reinstating Status=Active must be refused")
	}
	// No ReKey signatures at all → not a re-key record.
	if _, err := reKeyFixture(t, planetID, memberID, nil).VerifyReKeyQuorum(planetID, 0, founders, 0); err == nil {
		t.Error("a record with no ReKey CoSignatures must be refused")
	}
}

// TestVerifyReKeyQuorum_DIDKeyRefused pins the structural exclusion: a member
// whose MemberID is the SD-did-identity §2 fold of the key being retired
// cannot be re-keyed — quorum or no quorum.
func TestVerifyReKeyQuorum_DIDKeyRefused(t *testing.T) {
	planetID := tag.NewID()
	id1, pub1, prv1 := newSigner(t)
	founders := map[tag.UID]safe.PubKey{id1: pub1}

	// The subject is a did:key identity: MemberID = fold(prior signing key).
	kit, err := safe.CryptoKit(safe.Crypto.Poly25519.ID)
	if err != nil {
		t.Fatal(err)
	}
	subject := safe.KeyPair{Pub: safe.PubKey{CryptoKitID: safe.Crypto.Poly25519.ID, KeyType: safe.KeyType_SigningKey}}
	if err := kit.Signing.Generate(rand.Reader, &subject); err != nil {
		t.Fatal(err)
	}
	memberID, ok := amp.DIDKeyUID(safe.Crypto.Poly25519.ID, subject.Pub.Bytes)
	if !ok {
		t.Fatal("DIDKeyUID must fold an Ed25519 key")
	}

	record := reKeyFixture(t, planetID, memberID, nil)
	record.ReKeyPrior = signingKeyRef(subject.Pub.Bytes)
	digest, err := record.ReKeyCoSignatureDigest(planetID, 0)
	if err != nil {
		t.Fatal(err)
	}
	record.ReKey = []*amp.CoSignature{{MemberTag: amp.TagFromUID(id1), Signature: signDigest(t, digest, prv1)}}

	if _, err := record.VerifyReKeyQuorum(planetID, 0, founders, 0); err == nil {
		t.Error("a did:key member must be refused re-key even under a valid quorum")
	}
}

// TestDIDKeyUID_W3CVector pins the fold to the canonical Ed25519 example from
// the W3C-CCG did:key specification — an EXTERNAL vector, so a symmetric
// encoder bug cannot hide — and pins the folded UID as a golden byte fixture
// (identity is bytes).
func TestDIDKeyUID_W3CVector(t *testing.T) {
	pub, err := hex.DecodeString("2e6fcce36701dc791488e0d0b1745cc1e33a4c1c9fcc41c63bd343dbbe0970e6")
	if err != nil {
		t.Fatal(err)
	}
	uid, ok := amp.DIDKeyUID(safe.Crypto.Poly25519.ID, pub)
	if !ok {
		t.Fatal("Ed25519 (Poly25519) must have a did:key form")
	}
	// The fold of the W3C spec URI, and the same UID as a golden literal.
	if want := tag.HashName("did:key:z6MkhaXgBZDvotDkL5257faiztiGiC2QtKLGpbnnEGta2doK").ID; uid != want {
		t.Errorf("fold mismatch vs the W3C URI: got %v want %v", uid, want)
	}
	if golden := (tag.UID{0x4A51050DB678F155, 0x10526FC018DA0488}); uid != golden {
		t.Errorf("fold mismatch vs the golden UID bytes: got %#x", uid)
	}
	// A kit with no did:key form reads false, never a bogus fold.
	if _, ok := amp.DIDKeyUID(safe.CryptoKitID{}, pub); ok {
		t.Error("an unknown kit must have no did:key fold")
	}
	if _, ok := amp.DIDKeyUID(safe.Crypto.Poly25519.ID, pub[:31]); ok {
		t.Error("a non-32-byte key must have no did:key fold")
	}
}
