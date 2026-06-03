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
		CharterSchema: 1,
		PlanetID:      tagWithUID(0x3773, 888),
		GenesisEpoch:  tagWithUID(0xAA, 0xBB),
		ParentPlanet:  tagWithUID(0xCC, 0xDD),
		Origin: &amp.PlanetOrigin{
			FromPlanet:          tagWithUID(0xE1, 0xE2),
			FromEpoch:           tagWithUID(0xE3, 0xE4),
			FromChronicleHead_0: 0x1234,
			FromChronicleHead_1: 0x5678,
			ForkTime:            1717200000,
			Label:               "✙.בְּרֵאשִׁ֖ית.בָּרָ֣א.אֱלֹהִ֑ים.אֵ֥ת.הַשָּׁמַ֖יִם.וְאֵ֥ת.הָאָֽרֶץ.✙",
			Version:             1,
		},
		Privacy:                   amp.PrivacyMode_Public,
		Declaration:               &amp.Tags{Head: &amp.Tag{Text: "E pluribus unum"}},
		Founders:                  []*amp.Tag{tagWithUID(0xFFE, 0xFFF)},
		GenesisRequiredSignatures: 1,
	}
	terms := &amp.EpochTerms{
		TermsSchema:   1,
		EpochTag:      tagWithUID(0xAAAA, 0xBBBB),
		PreviousEpoch: tagWithUID(0x1111, 0x2222),
		EpochHeight:   7,
		CryptoKitID:   safe.CryptoKitID_P256,
		Label:         "AMP Genesis",
		Mark: &amp.BrandMark{
			AppName:    "Amplify",
			OrgName:    "art.media.platform",
			AppDomain:  "planet.tools",
			AppDesc:    "Hi Mom",
			URLSchemes: []string{"amp://"},
			NamedBy:    tagWithUID(0x90, 0xA0),
			Glyphs:     &amp.Tags{Head: tagWithUID(0x50, 0x60)},
		},
		Foyer: tagWithUID(0x10, 0x20),
		Index: &amp.Tag{
			UID_0:       0x30,
			UID_1:       0x40,
			I:           111,
			J:           222,
			K:           333,
			Units:       amp.Units_Meters,
			ContentType: "image/png",
			URI:         "amp://planet/index",
			Text:        "Index.Tag",
		},
		GovernanceGroup:    tagWithUID(0x70, 0x80),
		RequiredSignatures: 3,
		Seal:               amp.SealState_Paused,
		CodexEdition: &amp.Address{
			PlanetID_0: 0xA1, PlanetID_1: 0xA2,
			NodeID_0: 0xB1, NodeID_1: 0xB2,
			AttrID_0: 0xC1, AttrID_1: 0xC2,
			ItemID_0: 0xD1, ItemID_1: 0xD2,
			EditID_0: 0xE5, EditID_1: 0xE6,
		},
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
			MaxBlobBytesPerWindow:   500 << 20,
			BlobRateLimitWindowSecs: 3600,
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

	const goldenHex = "0000000000000137080112121973370000000000002178030000000000001a1219aa0000000000000021bb00000000000000221219cc0000000000000021dd000000000000002ac8010a1219e10000000000000021e200000000000000121219e30000000000000021e4000000000000001934120000000000002178560000000000002880c9e9b206328201e29c992ed791d6b0d6bcd7a8d6b5d790d7a9d6b4d781d696d799d7aa2ed791d6b8d6bcd7a8d6b8d6a3d7902ed790d6b1d79cd6b9d794d6b4d691d799d79d2ed790d6b5d6a5d7aa2ed794d6b7d7a9d6b8d6bcd781d79ed6b7d696d799d6b4d79d2ed795d6b0d790d6b5d6a5d7aa2ed794d6b8d790d6b8d6bdd7a8d6b6d7a52ee29c99a0060130073a140a12e2010f4520706c75726962757320756e756d421219fe0f00000000000021ff0f000000000000480100000000000001cf0801121219aaaa00000000000021bbbb0000000000001a121911110000000000002122220000000000002007280532202e1e990fb97bd9c79ee1e6c7d586b58da5f8fda6b49292a5337cd87d4597038d3802420b414d502047656e657369734a650a07416d706c69667912126172742e6d656469612e706c6174666f726d1a0c706c616e65742e746f6f6c7322064869204d6f6d2a06616d703a2f2f321219900000000000000021a0000000000000003a140a1219500000000000000021600000000000000052121910000000000000002120000000000000005a4d19300000000000000021400000000000000080016f8801de019001cd02a8010bc20109696d6167652f706e67d20112616d703a2f2f706c616e65742f696e646578e20109496e6465782e5461676212197000000000000000218000000000000000680370017a5a09a10000000000000011a20000000000000019b10000000000000021b20000000000000029c10000000000000031c20000000000000039d10000000000000041d20000000000000049e50000000000000051e60000000000000082012908808040108080803218904e2080a3052880f52430ac0238e80740644880f52450808080fa0158901c880180ceda03"

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
