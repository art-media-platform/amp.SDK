package amp_test

import (
	"crypto/rand"
	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/safe"
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

	sig, err := kit.Sign(canon, prv)
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
	sig, err := kit.Sign(canon, prv)
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

func makeTestEpoch(t *testing.T) *amp.PlanetEpoch {
	t.Helper()
	return &amp.PlanetEpoch{
		EpochTag:           amp.TagFromUID(tag.UID{100, 200}),
		Label:              "Genesis",
		CryptoKitID:        safe.CryptoKitID_Poly25519,
		RequiredSignatures: 0,
	}
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
	if err := kit.GenerateKey(rand.Reader, 0, &kp); err != nil {
		t.Fatal(err)
	}
	return kit, kp.Pub.Bytes, kp.Prv
}
