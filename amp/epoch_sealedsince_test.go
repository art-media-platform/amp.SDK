package amp_test

// The ContentSealedSince chain rule (AOM SD-security-sync.md §4.4, §7.7): a
// marker is introduced only equal to its epoch's own EpochTag and is carried
// unchanged by every successor; the planet-level answer ignores it on a
// Public Charter.

import (
	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// sealedSinceChain builds epochs of one charter: tagged by (hi, lo) and,
// when prev is set, chained to it at height + 1.
type sealedSinceChain struct {
	t       *testing.T
	charter *amp.PlanetCharter
}

func (chain sealedSinceChain) epoch(hi, lo uint64, prev *amp.PlanetEpoch, marker *amp.Tag) *amp.PlanetEpoch {
	chain.t.Helper()
	terms := &amp.EpochTerms{
		TermsSchema:        1,
		EpochTag:           &amp.Tag{UID_0: hi, UID_1: lo},
		ContentSealedSince: marker,
	}
	if prev != nil {
		prevTerms, err := prev.ParsedTerms()
		if err != nil {
			chain.t.Fatal(err)
		}
		terms.PreviousEpoch = prevTerms.EpochTag
		terms.EpochHeight = prevTerms.EpochHeight + 1
	}
	env, err := amp.AssembleEpoch(chain.charter, terms, safe.HashKitID_Blake2s_256)
	if err != nil {
		chain.t.Fatal(err)
	}
	return env
}

func TestVerifyCharterContinuity_ContentSealedSince(t *testing.T) {
	uid := func(hi, lo uint64) *amp.Tag { return &amp.Tag{UID_0: hi, UID_1: lo} }
	chain := sealedSinceChain{
		t: t,
		charter: &amp.PlanetCharter{
			CharterSchema: 1,
			PlanetID:      uid(0x5EA1, 0x5EA1),
			GenesisEpoch:  uid(100, 200),
			Privacy:       amp.PrivacyMode_Confidential,
		},
	}

	genesis := chain.epoch(100, 200, nil, uid(100, 200))
	if err := genesis.VerifyCharterContinuity(nil); err != nil {
		t.Fatalf("a genesis marking its own EpochTag refused: %v", err)
	}
	if got := genesis.ContentSealedSince(); got != (tag.UID{100, 200}) {
		t.Fatalf("Confidential genesis ContentSealedSince = %v, want its EpochTag", got)
	}
	if err := chain.epoch(100, 200, nil, uid(99, 1)).VerifyCharterContinuity(nil); err == nil {
		t.Error("a genesis marker other than its own EpochTag passed")
	}

	if err := chain.epoch(101, 201, genesis, uid(100, 200)).VerifyCharterContinuity(genesis); err != nil {
		t.Errorf("a rotation carrying the marker refused: %v", err)
	}
	if err := chain.epoch(101, 201, genesis, nil).VerifyCharterContinuity(genesis); err == nil {
		t.Error("a rotation dropping the marker passed")
	}
	if err := chain.epoch(101, 201, genesis, uid(101, 201)).VerifyCharterContinuity(genesis); err == nil {
		t.Error("a rotation changing the marker passed")
	}

	// A markerless chain: one rotation may introduce its own EpochTag (the
	// S4 rotation), never another value.
	unmarked := chain.epoch(100, 200, nil, nil)
	if err := chain.epoch(101, 201, unmarked, uid(101, 201)).VerifyCharterContinuity(unmarked); err != nil {
		t.Errorf("a rotation introducing its own EpochTag refused: %v", err)
	}
	if err := chain.epoch(101, 201, unmarked, uid(100, 200)).VerifyCharterContinuity(unmarked); err == nil {
		t.Error("a rotation introducing a backdated marker passed")
	}
	if got := unmarked.ContentSealedSince(); got.IsSet() {
		t.Errorf("markerless ContentSealedSince = %v, want nil", got)
	}

	public := sealedSinceChain{
		t: t,
		charter: &amp.PlanetCharter{
			CharterSchema: 1,
			PlanetID:      uid(0xB0B, 0xB0B),
			GenesisEpoch:  uid(100, 200),
			Privacy:       amp.PrivacyMode_Public,
		},
	}
	if got := public.epoch(100, 200, nil, uid(100, 200)).ContentSealedSince(); got.IsSet() {
		t.Errorf("Public ContentSealedSince = %v, want nil (inert)", got)
	}
	if got := amp.EpochFromTerms(&amp.EpochTerms{ContentSealedSince: uid(1, 2)}).ContentSealedSince(); got.IsSet() {
		t.Errorf("Charterless ContentSealedSince = %v, want nil", got)
	}
}
