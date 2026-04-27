package amp_test

import (
	"encoding/hex"
	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/safe"
)

// TestPlanetEpoch_CanonicalBytes_GoldenFixture is the regression guard for the
// canonical encoding used by PlanetEpoch.CoSignatures.  Encoding is produced by
// proto.MarshalOptions{Deterministic: true}, which is bit-stable across major
// protobuf implementations PROVIDED amp protos forbid `map<>` and `oneof`
// (see PRD-guidelines.md).
//
// The hex string below acts as the regression lock: any change to the
// PlanetEpoch schema or to the canonization logic that disturbs the byte layout
// fails this test, forcing a deliberate review.  At the moment we accept a
// real wire change, the hex is updated.
//
// IF this test fails after a schema edit, decide whether the edit is
// intentional.  If yes, regenerate the hex from the encoder output and commit.
// If no, fix the offending change.
func TestPlanetEpoch_CanonicalBytes_GoldenFixture(t *testing.T) {
	epoch := &amp.PlanetEpoch{
		EpochTag:           tagWithUID(100, 200),
		PreviousEpoch:      tagWithUID(1, 2),
		Label:              "Genesis",
		CryptoKitID:        safe.CryptoKitID_Poly25519,
		IsPublic:           false,
		MaxGracePeriod:     7776000, // 90 days in seconds
		Declaration:        "Test fixture for canonization freeze.",
		RequiredSignatures: 2,
	}

	got, err := epoch.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}

	const goldenHex = "0a1219640000000000000021c80000000000000012121901000000000000002102000000000000002a0747656e657369735001c80180ceda03c2022554657374206669787475726520666f722063616e6f6e697a6174696f6e20667265657a652ed80202"

	gotHex := hex.EncodeToString(got)
	if gotHex != goldenHex {
		t.Errorf("\ncanonical encoding drift detected\n got:  %s\nwant: %s\n\nIF this change was deliberate, every signature over canonical bytes from before this commit will fail to verify.  Update the golden hex only if the spec is being intentionally amended.",
			gotHex, goldenHex)
	}
}

// TestPlanetEpoch_CanonicalBytes_AllKnobs locks the encoding for a fully-
// populated PlanetEpoch — every governance field set, VaultOpts populated,
// Tags non-zero.  Proves: deterministic, tampering-detected, Signatures and
// Witnesses excluded.
func TestPlanetEpoch_CanonicalBytes_AllKnobs(t *testing.T) {
	epoch := &amp.PlanetEpoch{
		EpochTag:      tagWithUID(0xAAAA, 0xBBBB),
		PreviousEpoch: tagWithUID(0x1111, 0x2222),
		Label:         "Rotation 7",
		CryptoKitID:   safe.CryptoKitID_P256,
		IsPublic:      true,
		VaultOpts: &amp.VaultOpts{
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
		MaxGracePeriod:     90 * 86400,
		Foyer:              tagWithUID(0x10, 0x20),
		Index:              tagWithUID(0x30, 0x40),
		Glyph: &amp.Tags{
			Head: tagWithUID(0x50, 0x60),
		},
		About:              "Founders' communion planet.",
		Declaration:        "We bind ourselves to honest record across all our days.",
		GovernanceGroup:    tagWithUID(0x70, 0x80),
		RequiredSignatures: 3,
		LocalOnly:          false,
	}

	got, err := epoch.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}

	// Determinism: second call produces identical bytes.
	got2, err := epoch.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(got) != hex.EncodeToString(got2) {
		t.Fatal("CanonicalBytes must be deterministic across calls")
	}

	// Tampering detection: changing an included field changes the bytes.
	epoch.Label = "Rotation 8"
	tampered, err := epoch.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(got) == hex.EncodeToString(tampered) {
		t.Fatal("changing Label must change canonical bytes")
	}

	// Signatures + witnesses (excluded fields) do NOT affect bytes.
	epoch.Label = "Rotation 7" // restore
	epoch.Signatures = []*amp.CoSignature{
		{MemberTag: tagWithUID(1, 2), Signature: []byte{0xff, 0xfe}},
	}
	epoch.Witnesses = []*amp.CoSignature{
		{MemberTag: tagWithUID(3, 4), Signature: []byte{0xfd, 0xfc}},
	}
	withSigs, err := epoch.CanonicalBytes()
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(got) != hex.EncodeToString(withSigs) {
		t.Fatal("Signatures + Witnesses must NOT affect canonical bytes")
	}
}

func tagWithUID(hi, lo uint64) *amp.Tag {
	return &amp.Tag{UID_0: hi, UID_1: lo}
}
