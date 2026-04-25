package amp_test

import (
	"crypto/rand"
	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	_ "github.com/art-media-platform/amp.SDK/stdlib/safe/p256"
	_ "github.com/art-media-platform/amp.SDK/stdlib/safe/poly25519"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

func TestPlanetEpoch_CanonicalBytes_ExcludesSignatures(t *testing.T) {
	epoch := makeTestEpoch(t)

	canonEmpty, err := epoch.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}

	epoch.Signatures = []*amp.CoSignature{
		{MemberTag: amp.TagFromUID(tag.UID{1, 2}), Signature: []byte{0xde, 0xad, 0xbe, 0xef}},
	}

	canonWithSig, err := epoch.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}

	if string(canonEmpty) != string(canonWithSig) {
		t.Fatal("CanonicalBytes must be identical regardless of Signatures content")
	}
}

func TestPlanetEpoch_SignVerify_Roundtrip(t *testing.T) {
	epoch := makeTestEpoch(t)
	kit, pub, prv := freshKeyPair(t, safe.CryptoKitID_Poly25519)

	canon, err := epoch.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}

	sig, err := kit.Signing.Sign(canon, prv)
	if err != nil {
		t.Fatal(err)
	}

	cosig := &amp.CoSignature{
		MemberTag: amp.TagFromUID(tag.UID{42, 43}),
		Signature: sig,
	}

	if err := epoch.VerifyCoSignature(cosig, pub, safe.CryptoKitID_Poly25519); err != nil {
		t.Fatalf("verify should succeed: %v", err)
	}

	epoch.Signatures = []*amp.CoSignature{cosig}
	if err := epoch.VerifyCoSignature(cosig, pub, safe.CryptoKitID_Poly25519); err != nil {
		t.Fatalf("verify must still succeed after appending to Signatures: %v", err)
	}
}

func TestPlanetEpoch_Verify_RejectsTampered(t *testing.T) {
	epoch := makeTestEpoch(t)
	kit, pub, prv := freshKeyPair(t, safe.CryptoKitID_Poly25519)

	canon, err := epoch.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	sig, err := kit.Signing.Sign(canon, prv)
	if err != nil {
		t.Fatal(err)
	}
	cosig := &amp.CoSignature{
		MemberTag: amp.TagFromUID(tag.UID{42, 43}),
		Signature: sig,
	}

	epoch.Label = "Tampered"
	if err := epoch.VerifyCoSignature(cosig, pub, safe.CryptoKitID_Poly25519); err == nil {
		t.Fatal("verify must reject signature over a tampered epoch")
	}
}

func TestPlanetEpoch_Verify_RejectsEmptySig(t *testing.T) {
	epoch := makeTestEpoch(t)
	_, pub, _ := freshKeyPair(t, safe.CryptoKitID_Poly25519)

	err := epoch.VerifyCoSignature(&amp.CoSignature{
		MemberTag: amp.TagFromUID(tag.UID{42, 43}),
	}, pub, safe.CryptoKitID_Poly25519)
	if err == nil {
		t.Fatal("verify must reject empty signature")
	}
}

// TestPlanetEpoch_MixedSuiteQuorum is the load-bearing demonstration that a
// single genesis epoch can be co-signed by founders on different CryptoKits.
// One Poly25519 founder + one P-256 founder both sign the same canonical
// bytes; each signature is verified against its signer's native kit.
//
// This is the proof that PlanetEpoch.VerifyCoSignature is per-signer by design.
//
// The three-kit version (adding secp256k1 / wallet interop) lives in amp.planet
// where the secp256k1 kit is registered — see amp/apps/app.metamask.
func TestPlanetEpoch_MixedSuiteQuorum(t *testing.T) {
	epoch := makeTestEpoch(t)

	canon, err := epoch.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}

	polyKit, polyPub, polyPrv := freshKeyPair(t, safe.CryptoKitID_Poly25519)
	p256Kit, p256Pub, p256Prv := freshKeyPair(t, safe.CryptoKitID_P256)

	polySig, err := polyKit.Signing.Sign(canon, polyPrv)
	if err != nil {
		t.Fatal(err)
	}
	p256Sig, err := p256Kit.Signing.Sign(canon, p256Prv)
	if err != nil {
		t.Fatal(err)
	}

	polyCoSig := &amp.CoSignature{
		MemberTag: amp.TagFromUID(tag.UID{1, 1}),
		Signature: polySig,
	}
	p256CoSig := &amp.CoSignature{
		MemberTag: amp.TagFromUID(tag.UID{2, 2}),
		Signature: p256Sig,
	}

	// Each CoSignature is verified against its signer's own kit.  This is the
	// flow a vault controller runs when validating a multi-founder genesis.
	if err := epoch.VerifyCoSignature(polyCoSig, polyPub, safe.CryptoKitID_Poly25519); err != nil {
		t.Fatalf("Poly25519 cosignature must verify: %v", err)
	}
	if err := epoch.VerifyCoSignature(p256CoSig, p256Pub, safe.CryptoKitID_P256); err != nil {
		t.Fatalf("P-256 cosignature must verify: %v", err)
	}

	// Cross-kit swaps must fail: a Poly25519 signature cannot be verified as P-256,
	// and a P-256 signature cannot be verified as Poly25519.
	if err := epoch.VerifyCoSignature(polyCoSig, polyPub, safe.CryptoKitID_P256); err == nil {
		t.Fatal("Poly25519 signature must not verify under P-256 kit")
	}
	if err := epoch.VerifyCoSignature(p256CoSig, p256Pub, safe.CryptoKitID_Poly25519); err == nil {
		t.Fatal("P-256 signature must not verify under Poly25519 kit")
	}

	// Populating Signatures[] for both founders must not break verification —
	// CanonicalBytes() excludes Signatures from the signed payload.
	epoch.Signatures = []*amp.CoSignature{polyCoSig, p256CoSig}
	if err := epoch.VerifyCoSignature(polyCoSig, polyPub, safe.CryptoKitID_Poly25519); err != nil {
		t.Fatalf("Poly25519 verify after Signatures populated: %v", err)
	}
	if err := epoch.VerifyCoSignature(p256CoSig, p256Pub, safe.CryptoKitID_P256); err != nil {
		t.Fatalf("P-256 verify after Signatures populated: %v", err)
	}
}

// TestPlanetEpoch_Declaration_ParticipatesInSigning confirms that the
// human-readable Declaration is part of canonical bytes — tampering with the
// founders' stated intent must break verification, just like tampering with
// EpochTag or GovernanceGroup.  This is what makes the declaration load-bearing
// rather than decorative.
func TestPlanetEpoch_Declaration_ParticipatesInSigning(t *testing.T) {
	epoch := makeTestEpoch(t)
	epoch.Declaration = "We, Alice and Bob, found this planet for our shared art practice."

	kit, pub, prv := freshKeyPair(t, safe.CryptoKitID_Poly25519)
	canon, err := epoch.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	sig, err := kit.Signing.Sign(canon, prv)
	if err != nil {
		t.Fatal(err)
	}
	cosig := &amp.CoSignature{
		MemberTag: amp.TagFromUID(tag.UID{42, 43}),
		Signature: sig,
	}

	if err := epoch.VerifyCoSignature(cosig, pub, safe.CryptoKitID_Poly25519); err != nil {
		t.Fatalf("verify should succeed on untampered epoch: %v", err)
	}

	epoch.Declaration = "We, Alice and Bob, found this planet for money laundering."
	if err := epoch.VerifyCoSignature(cosig, pub, safe.CryptoKitID_Poly25519); err == nil {
		t.Fatal("verify must reject signature after Declaration tamper")
	}
}

// TestPlanetEpoch_Witnesses_ExcludedFromCanonicalBytes confirms that appending
// witnesses after-the-fact does not invalidate the founders' quorum signatures.
// Witnesses attest; they do not re-open the signing payload.
func TestPlanetEpoch_Witnesses_ExcludedFromCanonicalBytes(t *testing.T) {
	epoch := makeTestEpoch(t)

	canonBefore, err := epoch.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}

	epoch.Witnesses = []*amp.CoSignature{
		{MemberTag: amp.TagFromUID(tag.UID{99, 1}), Signature: []byte{0xaa, 0xbb}},
		{MemberTag: amp.TagFromUID(tag.UID{99, 2}), Signature: []byte{0xcc, 0xdd}},
	}

	canonAfter, err := epoch.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}

	if string(canonBefore) != string(canonAfter) {
		t.Fatal("CanonicalBytes must be identical regardless of Witnesses content")
	}
}

// TestPlanetEpoch_Witnesses_VerifyOverSameCanonicalBytes confirms that a
// witness signs the exact same payload as a quorum signer — the role
// distinction (voting vs. attesting) lives in which slice the signature lands,
// not in a different signed message.  A notary, officiant, or AI monitor's
// signature can be validated with the same VerifyCoSignature call that
// validates a founder's.
func TestPlanetEpoch_Witnesses_VerifyOverSameCanonicalBytes(t *testing.T) {
	epoch := makeTestEpoch(t)
	epoch.Declaration = "Alice and Bob co-found, witnessed by Carol the notary."

	kit, pub, prv := freshKeyPair(t, safe.CryptoKitID_Poly25519)
	canon, err := epoch.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	witnessSig, err := kit.Signing.Sign(canon, prv)
	if err != nil {
		t.Fatal(err)
	}

	witness := &amp.CoSignature{
		MemberTag: amp.TagFromUID(tag.UID{77, 77}),
		Signature: witnessSig,
	}

	epoch.Witnesses = []*amp.CoSignature{witness}

	// The witness's signature verifies by the same mechanism as a founder's —
	// the only difference is the slice it lives in.
	if err := epoch.VerifyCoSignature(witness, pub, safe.CryptoKitID_Poly25519); err != nil {
		t.Fatalf("witness signature must verify under the same canonical bytes: %v", err)
	}
}

func makeTestEpoch(t *testing.T) *amp.PlanetEpoch {
	t.Helper()
	return &amp.PlanetEpoch{
		EpochTag:           amp.TagFromUID(tag.UID{100, 200}),
		Label:              "Genesis",
		CryptoKitID:        safe.CryptoKitID_Poly25519,
		RequiredSignatures: 0,
	}
}

func freshKeyPair(t *testing.T, kitID safe.CryptoKitID) (*safe.KitSpec, []byte, []byte) {
	t.Helper()
	kit, err := safe.GetKit(kitID)
	if err != nil {
		t.Fatal(err)
	}
	kp := safe.KeyPair{
		Pub: safe.PubKey{
			CryptoKitID: kitID,
			KeyType:     safe.KeyType_SigningKey,
		},
	}
	if err := kit.Signing.Generate(rand.Reader, &kp); err != nil {
		t.Fatal(err)
	}
	return kit, kp.Pub.Bytes, kp.Prv
}
