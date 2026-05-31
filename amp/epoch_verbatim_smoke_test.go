package amp_test

import (
	"crypto/rand"
	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"google.golang.org/protobuf/proto"
)

// TestEpochVerbatim_Roundtrip exercises the three-layer verbatim-signed model end
// to end: assemble (marshal once) -> sign the FRAME -> verify -> tamper-detect ->
// charter continuity across a rotation -> Terminal-seal rejection.  This proves
// the sign/verify mechanics actually work, not just compile.
func TestEpochVerbatim_Roundtrip(t *testing.T) {
	kitID := safe.CryptoKitID_Poly25519
	kit, err := safe.GetKit(kitID)
	if err != nil {
		t.Fatal(err)
	}

	uid := func(hi, lo uint64) *amp.Tag { return &amp.Tag{UID_0: hi, UID_1: lo} }

	charter := &amp.PlanetCharter{
		CharterSchema:             1,
		PlanetID:                  uid(0xABCD, 0xEF01),
		GenesisEpoch:              uid(100, 200),
		Privacy:                   amp.PrivacyMode_Confidential,
		Declaration:               &amp.Tags{Head: uid(1, 1)},
		Founders:                  []*amp.Tag{uid(0xF1, 0)},
		GenesisRequiredSignatures: 1,
	}
	genesisTerms := &amp.EpochTerms{
		TermsSchema: 1,
		EpochTag:    uid(100, 200), // == GenesisEpoch
		EpochHeight: 0,
		CryptoKitID: kitID,
		Label:       "Genesis",
		Mark:        &amp.BrandMark{AppName: "Pentagon"},
	}

	// 1) Assemble: marshal charter+terms once, bind CharterHash into Terms.
	env, err := amp.AssembleEpoch(charter, genesisTerms, safe.HashKitID_Blake2s_256)
	if err != nil {
		t.Fatalf("AssembleEpoch: %v", err)
	}

	// 2) Sign the FRAME (the stored bytes), append a CoSignature.
	frame, err := env.SignedBytes()
	if err != nil {
		t.Fatalf("SignedBytes: %v", err)
	}
	kp := safe.KeyPair{Pub: safe.PubKey{CryptoKitID: kitID, KeyType: safe.KeyType_SigningKey}}
	if err := kit.Signing.Generate(rand.Reader, &kp); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	sig, err := kit.Signing.Sign(frame, kp.Prv)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	pub := kp.Pub.Bytes
	cosig := &amp.CoSignature{MemberTag: uid(0xF1, 0), Signature: sig}

	// 3) Verify succeeds over the stored bytes.
	if err := env.VerifyCoSignature(cosig, pub, kitID); err != nil {
		t.Fatalf("VerifyCoSignature (valid): %v", err)
	}

	// 4) Tamper a Charter byte -> verify must fail (signed-bytes integrity).
	bad := proto.Clone(env).(*amp.PlanetEpoch)
	bad.Charter[len(bad.Charter)-1] ^= 0xFF
	if err := bad.VerifyCoSignature(cosig, pub, kitID); err == nil {
		t.Fatal("VerifyCoSignature accepted a tampered Charter")
	}

	// 5) Genesis charter continuity: CharterHash matches carried Charter; no prev.
	if err := env.VerifyCharterContinuity(nil); err != nil {
		t.Fatalf("genesis VerifyCharterContinuity: %v", err)
	}

	// 6) Rotation: same charter bytes, height+1, PreviousEpoch -> genesis.
	rotTerms := &amp.EpochTerms{
		TermsSchema:   1,
		EpochTag:      uid(101, 201),
		PreviousEpoch: uid(100, 200),
		EpochHeight:   1,
		CryptoKitID:   kitID,
		Label:         "Rotation 1",
		Mark:          &amp.BrandMark{AppName: "Pentagon"},
	}
	rot, err := amp.AssembleEpoch(charter, rotTerms, safe.HashKitID_Blake2s_256)
	if err != nil {
		t.Fatalf("AssembleEpoch(rot): %v", err)
	}
	if err := rot.VerifyCharterContinuity(env); err != nil {
		t.Fatalf("rotation VerifyCharterContinuity: %v", err)
	}

	// 7) Terminal seal the genesis epoch -> no successor may chain off it.
	sealedTerms := proto.Clone(genesisTerms).(*amp.EpochTerms)
	sealedTerms.Seal = amp.SealState_Sealed
	sealed, err := amp.AssembleEpoch(charter, sealedTerms, safe.HashKitID_Blake2s_256)
	if err != nil {
		t.Fatalf("AssembleEpoch(sealed): %v", err)
	}
	if err := rot.VerifyCharterContinuity(sealed); err == nil {
		t.Fatal("VerifyCharterContinuity accepted a successor chaining off a Terminal epoch")
	}

	t.Log("verbatim epoch: assemble/sign/verify/tamper/continuity/terminal-seal all OK")
}
