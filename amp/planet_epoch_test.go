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

func TestPlanetEpoch_SignedBytes_ExcludesSignatures(t *testing.T) {
	epoch := makeTestEpoch(t)

	frameEmpty, err := epoch.SignedBytes()
	if err != nil {
		t.Fatal(err)
	}

	epoch.Signatures = []*amp.CoSignature{
		{MemberTag: amp.TagFromUID(tag.UID{1, 2}), Signature: []byte{0xde, 0xad, 0xbe, 0xef}},
	}

	frameWithSig, err := epoch.SignedBytes()
	if err != nil {
		t.Fatal(err)
	}

	if string(frameEmpty) != string(frameWithSig) {
		t.Fatal("SignedBytes must be identical regardless of Signatures content")
	}
}

func TestPlanetEpoch_SignVerify_Roundtrip(t *testing.T) {
	epoch := makeTestEpoch(t)
	kit, pub, prv := freshKeyPair(t, safe.CryptoKitID_Poly25519)

	frame, err := epoch.SignedBytes()
	if err != nil {
		t.Fatal(err)
	}

	sig, err := kit.Signing.Sign(frame, prv)
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

	frame, err := epoch.SignedBytes()
	if err != nil {
		t.Fatal(err)
	}
	sig, err := kit.Signing.Sign(frame, prv)
	if err != nil {
		t.Fatal(err)
	}
	cosig := &amp.CoSignature{
		MemberTag: amp.TagFromUID(tag.UID{42, 43}),
		Signature: sig,
	}

	// Flip a byte inside the verbatim Terms — the FRAME no longer matches what
	// was signed, so verification must fail.
	epoch.Terms[len(epoch.Terms)-1] ^= 0xFF
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
// One Poly25519 founder + one P-256 founder both sign the same FRAME; each
// signature is verified against its signer's native kit.
//
// This is the proof that PlanetEpoch.VerifyCoSignature is per-signer by design.
//
// The three-kit version (adding secp256k1 / wallet interop) lives in amp.planet
// where the secp256k1 kit is registered — see amp/apps/app.evmwallet.
func TestPlanetEpoch_MixedSuiteQuorum(t *testing.T) {
	epoch := makeTestEpoch(t)

	frame, err := epoch.SignedBytes()
	if err != nil {
		t.Fatal(err)
	}

	polyKit, polyPub, polyPrv := freshKeyPair(t, safe.CryptoKitID_Poly25519)
	p256Kit, p256Pub, p256Prv := freshKeyPair(t, safe.CryptoKitID_P256)

	polySig, err := polyKit.Signing.Sign(frame, polyPrv)
	if err != nil {
		t.Fatal(err)
	}
	p256Sig, err := p256Kit.Signing.Sign(frame, p256Prv)
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
	// the FRAME excludes Signatures from the signed payload.
	epoch.Signatures = []*amp.CoSignature{polyCoSig, p256CoSig}
	if err := epoch.VerifyCoSignature(polyCoSig, polyPub, safe.CryptoKitID_Poly25519); err != nil {
		t.Fatalf("Poly25519 verify after Signatures populated: %v", err)
	}
	if err := epoch.VerifyCoSignature(p256CoSig, p256Pub, safe.CryptoKitID_P256); err != nil {
		t.Fatalf("P-256 verify after Signatures populated: %v", err)
	}
}

// TestPlanetEpoch_Declaration_ParticipatesInSigning confirms that the founders'
// human-readable Declaration is part of the signed Charter bytes — swapping the
// founders' stated intent must break verification, just like tampering with
// EpochTag or GovernanceGroup.  This is what makes the declaration load-bearing
// rather than decorative.
func TestPlanetEpoch_Declaration_ParticipatesInSigning(t *testing.T) {
	mkEnv := func(declaration string) *amp.PlanetEpoch {
		t.Helper()
		charter := &amp.PlanetCharter{
			CharterSchema: 1,
			PlanetID:      amp.TagFromUID(tag.UID{0xABCD, 0xEF01}),
			GenesisEpoch:  amp.TagFromUID(tag.UID{100, 200}),
			Declaration:   &amp.Tags{Head: &amp.Tag{Text: declaration}},
		}
		terms := &amp.EpochTerms{
			TermsSchema: 1,
			EpochTag:    amp.TagFromUID(tag.UID{100, 200}),
			CryptoKitID: safe.CryptoKitID_Poly25519,
		}
		env, err := amp.AssembleEpoch(charter, terms, safe.HashKitID_Blake2s_256)
		if err != nil {
			t.Fatal(err)
		}
		return env
	}

	honest := mkEnv("We, Alice and Bob, found this planet for our shared art practice.")
	kit, pub, prv := freshKeyPair(t, safe.CryptoKitID_Poly25519)
	frame, err := honest.SignedBytes()
	if err != nil {
		t.Fatal(err)
	}
	sig, err := kit.Signing.Sign(frame, prv)
	if err != nil {
		t.Fatal(err)
	}
	cosig := &amp.CoSignature{
		MemberTag: amp.TagFromUID(tag.UID{42, 43}),
		Signature: sig,
	}

	if err := honest.VerifyCoSignature(cosig, pub, safe.CryptoKitID_Poly25519); err != nil {
		t.Fatalf("verify should succeed on the honest declaration: %v", err)
	}

	// The honest founders' signature must not validate an envelope whose
	// Declaration was swapped — the intent lives inside the signed Charter.
	forged := mkEnv("We, Alice and Bob, found this planet for money laundering.")
	if err := forged.VerifyCoSignature(cosig, pub, safe.CryptoKitID_Poly25519); err == nil {
		t.Fatal("verify must reject the honest signature against a swapped Declaration")
	}
}

// TestPlanetEpoch_Witnesses_ExcludedFromFrame confirms that appending witnesses
// after-the-fact does not invalidate the founders' quorum signatures.
// Witnesses attest; they do not re-open the signing payload.
func TestPlanetEpoch_Witnesses_ExcludedFromFrame(t *testing.T) {
	epoch := makeTestEpoch(t)

	frameBefore, err := epoch.SignedBytes()
	if err != nil {
		t.Fatal(err)
	}

	epoch.Witnesses = []*amp.CoSignature{
		{MemberTag: amp.TagFromUID(tag.UID{99, 1}), Signature: []byte{0xaa, 0xbb}},
		{MemberTag: amp.TagFromUID(tag.UID{99, 2}), Signature: []byte{0xcc, 0xdd}},
	}

	frameAfter, err := epoch.SignedBytes()
	if err != nil {
		t.Fatal(err)
	}

	if string(frameBefore) != string(frameAfter) {
		t.Fatal("SignedBytes must be identical regardless of Witnesses content")
	}
}

// TestPlanetEpoch_Witnesses_VerifyOverSameFrame confirms that a witness signs
// the exact same payload as a quorum signer — the role distinction (voting vs.
// attesting) lives in which slice the signature lands, not in a different signed
// message.  A notary, officiant, or AI monitor's signature can be validated with
// the same VerifyCoSignature call that validates a founder's.
func TestPlanetEpoch_Witnesses_VerifyOverSameFrame(t *testing.T) {
	epoch := makeTestEpoch(t)

	kit, pub, prv := freshKeyPair(t, safe.CryptoKitID_Poly25519)
	frame, err := epoch.SignedBytes()
	if err != nil {
		t.Fatal(err)
	}
	witnessSig, err := kit.Signing.Sign(frame, prv)
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
		t.Fatalf("witness signature must verify under the same FRAME: %v", err)
	}
}

// makeTestEpoch assembles a minimal genesis envelope (Charter + Terms marshaled
// once, CharterHash bound) ready for co-signing over its FRAME.
func makeTestEpoch(t *testing.T) *amp.PlanetEpoch {
	t.Helper()
	charter := &amp.PlanetCharter{
		CharterSchema: 1,
		PlanetID:      amp.TagFromUID(tag.UID{0xABCD, 0xEF01}),
		GenesisEpoch:  amp.TagFromUID(tag.UID{100, 200}),
	}
	terms := &amp.EpochTerms{
		TermsSchema: 1,
		EpochTag:    amp.TagFromUID(tag.UID{100, 200}),
		Label:       "Genesis",
		CryptoKitID: safe.CryptoKitID_Poly25519,
	}
	env, err := amp.AssembleEpoch(charter, terms, safe.HashKitID_Blake2s_256)
	if err != nil {
		t.Fatal(err)
	}
	return env
}

func freshKeyPair(t *testing.T, kitID safe.CryptoKitID) (*safe.CryptoKit, []byte, []byte) {
	t.Helper()
	kit, err := safe.GetCryptoKit(kitID)
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
