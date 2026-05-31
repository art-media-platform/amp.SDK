package amp_test

import (
	"encoding/hex"
	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/safe"
)

// goldenEpoch assembles a fully-populated genesis envelope — every Charter and
// EpochTerms field a verifier reads, plus a populated VaultConfig and BrandMark —
// marshaled once via AssembleEpoch so the FRAME below is the verbatim signed
// artifact.  Shared by the golden-layout and exclusion regression guards.
func goldenEpoch(t *testing.T) *amp.PlanetEpoch {
	t.Helper()
	charter := &amp.PlanetCharter{
		CharterSchema:             1,
		PlanetID:                  tagWithUID(0x3773, 7337),
		GenesisEpoch:              tagWithUID(141592653589, 79323846264338),
		Privacy:                   amp.PrivacyMode_Public,
		Declaration:               &amp.Tags{Head: &amp.Tag{Text: "E pluribus unum"}},
		Founders:                  []*amp.Tag{tagWithUID(0xF1, 0), tagWithUID(0xF2, 0)},
		GenesisRequiredSignatures: 2,
	}
	terms := &amp.EpochTerms{
		TermsSchema:   1,
		EpochTag:      tagWithUID(0xAAAA, 0xBBBB),
		PreviousEpoch: tagWithUID(0x1111, 0x2222),
		EpochHeight:   7,
		CryptoKitID:   safe.CryptoKitID_P256,
		Label:         "✙.בְּרֵאשִׁ֖ית.בָּרָ֣א.אֱלֹהִ֑ים.אֵ֥ת.הַשָּׁמַ֖יִם.וְאֵ֥ת.הָאָֽרֶץ.✙",
		Mark: &amp.BrandMark{
			AppName:    "AMP",
			OrgName:    "art.media.platform",
			AppDomain:  "planet.tools",
			AppDesc:    "Hi Mom!",
			URLSchemes: []string{"amp://"},
			Glyphs:     &amp.Tags{Head: tagWithUID(0x50, 0x60)},
		},
		Foyer:              tagWithUID(0x10, 0x20),
		Index:              tagWithUID(0x30, 0x40),
		GovernanceGroup:    tagWithUID(0x70, 0x80),
		RequiredSignatures: 3,
		Seal:               amp.SealState_Paused,
		VaultConfig: &amp.VaultConfig{
			MaxTxMsgSize:            1 << 20,
			MaxBytesPerWindow:       100 << 20,
			MaxTxPerWindow:          10000,
			RateLimitWindowSecs:     86400,
			QuarantineRetentionSecs: 7 * 86400,
			MaxFutureSkewSecs:       300,
			MaxPendingPerEpoch:      1000,
			MaxPendingEpochs:        100,
			BootstrapTTLSecs:        7 * 86400,
		},
		MaxGracePeriod: 90 * 86400,
	}
	env, err := amp.AssembleEpoch(charter, terms, safe.HashKitID_Blake2s_256)
	if err != nil {
		t.Fatal(err)
	}
	return env
}

// TestPlanetEpoch_Frame_GoldenFixture is the regression guard for the verbatim
// FRAME layout that PlanetEpoch.CoSignatures cover:
//
//	u64BE(len Charter) || Charter || u64BE(len Terms) || Terms
//
// Authority does not depend on cross-language proto-marshal stability — each
// layer is marshaled once and signed/verified as the stored bytes.  This golden
// hex still locks the LAYOUT (field numbers + framing) so an accidental field
// renumber or framing change fails loudly and forces a deliberate review.
//
// IF this test fails after a schema edit, decide whether the edit is intentional.
// If yes, regenerate the hex from the encoder output and commit.  If no, fix the
// offending change.
func TestPlanetEpoch_Frame_GoldenFixture(t *testing.T) {
	env := goldenEpoch(t)

	frame, err := env.SignedBytes()
	if err != nil {
		t.Fatal(err)
	}

	const goldenHex = "000000000000005a0801121219733700000000000021a91c0000000000001a1219156f94f7200000002112c6ad052548000030073a140a12e2010f4520706c75726962757320756e756d420919f100000000000000420919f200000000000000480200000000000001900801121219aaaa00000000000021bbbb0000000000001a121911110000000000002122220000000000002007280532205f9f6cd78388087307e6200ca8fba1dfb73e04a03bf9649f20a7160c97537dbf3802428201e29c992ed791d6b0d6bcd7a8d6b5d790d7a9d6b4d781d696d799d7aa2ed791d6b8d6bcd7a8d6b8d6a3d7902ed790d6b1d79cd6b9d794d6b4d691d799d79d2ed790d6b5d6a5d7aa2ed794d6b7d7a9d6b8d6bcd781d79ed6b7d696d799d6b4d79d2ed795d6b0d790d6b5d6a5d7aa2ed794d6b8d790d6b8d6bdd7a8d6b6d7a52ee29c994a4e0a03414d5012126172742e6d656469612e706c6174666f726d1a0c706c616e65742e746f6f6c7322074869204d6f6d212a06616d703a2f2f3a140a1219500000000000000021600000000000000052121910000000000000002120000000000000005a1219300000000000000021400000000000000062121970000000000000002180000000000000006803700182012008808040108080803218904e2080a3052880f52430ac0238e80740644880f524880180ceda03"

	gotHex := hex.EncodeToString(frame)
	if gotHex != goldenHex {
		t.Errorf("\nFRAME layout drift detected\n got:  %s\nwant: %s\n\nIF this change was deliberate, every signature over the FRAME from before this commit will fail to verify.  Update the golden hex only if the layout is being intentionally amended.",
			gotHex, goldenHex)
	}
}

// TestPlanetEpoch_Frame_ExcludesCoSignatures proves the FRAME is stable under the
// fields meant to grow after signing (Signatures, Witnesses) and breaks under a
// byte tamper of a signed layer.
func TestPlanetEpoch_Frame_ExcludesCoSignatures(t *testing.T) {
	env := goldenEpoch(t)

	frame, err := env.SignedBytes()
	if err != nil {
		t.Fatal(err)
	}

	// Re-reading the stored bytes yields the identical FRAME.
	frame2, err := env.SignedBytes()
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(frame) != hex.EncodeToString(frame2) {
		t.Fatal("SignedBytes must be stable across calls")
	}

	// Signatures + Witnesses (appended after signing) do NOT affect the FRAME.
	env.Signatures = []*amp.CoSignature{
		{MemberTag: tagWithUID(1, 2), Signature: []byte{0xff, 0xfe}},
	}
	env.Witnesses = []*amp.CoSignature{
		{MemberTag: tagWithUID(3, 4), Signature: []byte{0xfd, 0xfc}},
	}
	withSigs, err := env.SignedBytes()
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(frame) != hex.EncodeToString(withSigs) {
		t.Fatal("Signatures + Witnesses must NOT affect the FRAME")
	}

	// Tampering a signed layer changes the FRAME.
	env.Terms[len(env.Terms)-1] ^= 0xFF
	tampered, err := env.SignedBytes()
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(frame) == hex.EncodeToString(tampered) {
		t.Fatal("tampering Terms must change the FRAME")
	}
}

func tagWithUID(hi, lo uint64) *amp.Tag {
	return &amp.Tag{UID_0: hi, UID_1: lo}
}
