package amp

import (
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// A Charter that does not parse is refused by VerifyGenesis even when the
// Terms carry its hash (the parse error is never swallowed).
func TestVerifyGenesis_MalformedCharterRefused(t *testing.T) {
	epochID := tag.UID{100, 200}
	terms := &EpochTerms{
		TermsSchema: 1,
		EpochTag:    TagFromUID(epochID),
	}
	epoch, err := assembleLayers([]byte{0xFF, 0xFF, 0xFF}, terms, safe.HashKitID_Blake2s_256)
	if err != nil {
		t.Fatal(err)
	}
	if err := epoch.VerifyCharterContinuity(nil); err != nil {
		t.Fatalf("the hash binds the malformed bytes; continuity should hold: %v", err)
	}
	if err := epoch.VerifyGenesis(tag.UID{1, 2}); err == nil {
		t.Fatal("VerifyGenesis accepted a Charter that does not parse")
	}
}
