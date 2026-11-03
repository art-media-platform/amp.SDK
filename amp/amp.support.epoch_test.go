package amp_test

import (
	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// The era boundary (SD-security-sync §4.4): VerifyGenesis refuses a Charter
// whose RequiredFeatures omits an EraRequiredFeatures entry, names a feature
// this binary lacks, or names CharterFeatureUnset.

var (
	eraPlanetID = tag.UID{0xABCD, 0xEF01}
	eraEpochID  = tag.UID{100, 200}
)

// eraGenesis assembles a founding epoch whose Charter carries features.
func eraGenesis(t *testing.T, features []amp.CharterFeature) *amp.PlanetEpoch {
	t.Helper()
	charter := &amp.PlanetCharter{
		CharterSchema:    1,
		PlanetID:         amp.TagFromUID(eraPlanetID),
		GenesisEpoch:     amp.TagFromUID(eraEpochID),
		RequiredFeatures: features,
	}
	terms := &amp.EpochTerms{
		TermsSchema:   1,
		EpochTag:      amp.TagFromUID(eraEpochID),
		Label:         "Genesis",
		CryptoKitID_0: safe.Crypto.Poly25519.ID[0],
		CryptoKitID_1: safe.Crypto.Poly25519.ID[1],
	}
	epoch, err := amp.AssembleEpoch(charter, terms, safe.HashKitID_Blake2s_256)
	if err != nil {
		t.Fatal(err)
	}
	return epoch
}

func TestVerifyGenesis_EraFeatures(t *testing.T) {
	for _, row := range []struct {
		name     string
		features []amp.CharterFeature
		accept   bool
	}{
		{"FounderAdmission accepted", amp.EraRequiredFeatures, true},
		{"duplicates normalize", []amp.CharterFeature{
			amp.CharterFeature_FounderAdmission,
			amp.CharterFeature_FounderAdmission,
		}, true},
		{"no RequiredFeatures refused", nil, false},
		{"unknown feature 999 refused", []amp.CharterFeature{
			amp.CharterFeature_FounderAdmission,
			amp.CharterFeature(999),
		}, false},
		{"CharterFeatureUnset refused", []amp.CharterFeature{
			amp.CharterFeature_FounderAdmission,
			amp.CharterFeature_CharterFeatureUnset,
		}, false},
	} {
		err := eraGenesis(t, row.features).VerifyGenesis(eraPlanetID)
		if row.accept && err != nil {
			t.Errorf("%s: VerifyGenesis refused: %v", row.name, err)
		}
		if !row.accept && err == nil {
			t.Errorf("%s: VerifyGenesis accepted", row.name)
		}
	}
}

// Stripping the marker from a signed genesis changes the signed frame: the
// founder's co-signature no longer verifies and VerifyGenesis refuses the
// Charter (the marker is inside the signed bytes, never beside them).
func TestVerifyGenesis_StrippedMarkerFailsSignature(t *testing.T) {
	signed := eraGenesis(t, amp.EraRequiredFeatures)
	kit, pub, prv := freshKeyPair(t, safe.Crypto.Poly25519.ID)
	digest, err := signed.CoSignatureDigest()
	if err != nil {
		t.Fatal(err)
	}
	sig, err := kit.Signing.Sign(digest, prv)
	if err != nil {
		t.Fatal(err)
	}
	cosig := &amp.CoSignature{
		MemberTag: amp.TagFromUID(tag.UID{42, 43}),
		Signature: sig,
	}
	if err := signed.VerifyCoSignature(cosig, pub, safe.Crypto.Poly25519.ID); err != nil {
		t.Fatalf("the signed genesis does not verify: %v", err)
	}
	if err := signed.VerifyGenesis(eraPlanetID); err != nil {
		t.Fatalf("the signed genesis is refused: %v", err)
	}

	stripped := eraGenesis(t, nil)
	if err := stripped.VerifyCoSignature(cosig, pub, safe.Crypto.Poly25519.ID); err == nil {
		t.Fatal("the co-signature verifies over a Charter with the marker stripped")
	}
	if err := stripped.VerifyGenesis(eraPlanetID); err == nil {
		t.Fatal("VerifyGenesis accepts a Charter with the marker stripped")
	}
}

// ParsePublicTxHeader reads the author from the payload's TxHeader (FromID
// is not an envelope field) and refuses a sealed tx.
func TestParsePublicTxHeader(t *testing.T) {
	fromID := tag.UID{0x5555, 0x6666}
	planetID := tag.UID{0x1111, 0x2222}
	tx := amp.TxNew()
	tx.SetTxID(tag.NewID())
	tx.SetPlanetID(planetID)
	tx.SetFromID(fromID)
	op := amp.TxOp{}
	op.Addr.NodeID = amp.HeadNodeID
	op.Addr.AttrID = tag.UID{9, 9}
	op.Addr.ItemID = fromID
	op.Flags = amp.TxOpFlags_Upsert
	tx.MarshalOpAndData(&op, []byte("body"))
	wire := []byte(nil)
	if err := amp.SealTx(tx, nil, &wire); err != nil {
		t.Fatal(err)
	}

	env, header, err := amp.ParsePublicTxHeader(wire)
	if err != nil {
		t.Fatal(err)
	}
	if env.PlanetID() != planetID {
		t.Errorf("envelope PlanetID = %v, want %v", env.PlanetID(), planetID)
	}
	if header.FromID() != fromID {
		t.Errorf("header FromID = %v, want %v", header.FromID(), fromID)
	}

	sealed := amp.TxNew()
	sealed.SetTxID(tag.NewID())
	sealed.SetPlanetID(planetID)
	sealed.SetEpochID(tag.NewID())
	sealed.SetFromID(fromID)
	sealedWire := []byte(nil)
	if err := amp.SealTx(sealed, nil, &sealedWire); err != nil {
		t.Fatal(err)
	}
	if _, _, err := amp.ParsePublicTxHeader(sealedWire); err == nil {
		t.Error("a sealed tx yielded a public header")
	}
	if _, _, err := amp.ParsePublicTxHeader(wire[:8]); err == nil {
		t.Error("a truncated frame parsed")
	}
}
