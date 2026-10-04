package std

import (
	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
)

// TestBlobProgressBands: every declared state value sits in the band its name claims
// (waiting 1–15, held 16–23, terminal 24–31), and an undeclared value is read by its band
// alone — a newer host's state a client has no name for still renders as waiting, held, or
// final (AD-app-www §5.6).
func TestBlobProgressBands(t *testing.T) {
	cases := []struct {
		name  string
		value int32
		want  BlobStateBand
	}{
		{"PullUnset", int32(amp.BlobPullState_PullUnset), BlobStateBand_Unset},
		{"Queued", int32(amp.BlobPullState_Queued), BlobStateBand_Waiting},
		{"AwaitingMeta", int32(amp.BlobPullState_AwaitingMeta), BlobStateBand_Waiting},
		{"Pulling", int32(amp.BlobPullState_Pulling), BlobStateBand_Waiting},
		{"NoSource", int32(amp.BlobPullState_NoSource), BlobStateBand_Held},
		{"Stalled", int32(amp.BlobPullState_Stalled), BlobStateBand_Held},
		{"PullFailed", int32(amp.BlobPullState_PullFailed), BlobStateBand_Terminal},
		{"PullComplete", int32(amp.BlobPullState_PullComplete), BlobStateBand_Terminal},
		{"NoReader", int32(amp.BlobReaderState_NoReader), BlobStateBand_Waiting},
		{"Reading", int32(amp.BlobReaderState_Reading), BlobStateBand_Waiting},
		{"Starving", int32(amp.BlobReaderState_Starving), BlobStateBand_Held},
		{"NoPeer", int32(amp.BlobPullReason_NoPeer), BlobStateBand_Held},
		{"SourcesBusy", int32(amp.BlobPullReason_SourcesBusy), BlobStateBand_Held},
		{"OverCap", int32(amp.BlobPullReason_OverCap), BlobStateBand_Terminal},
		{"ContentMismatch", int32(amp.BlobPullReason_ContentMismatch), BlobStateBand_Terminal},
		{"NoEpochKey", int32(amp.BlobPullReason_NoEpochKey), BlobStateBand_Terminal},
		{"BudgetStop", int32(amp.BlobPullReason_BudgetStop), BlobStateBand_Terminal},
		{"undeclared waiting", 9, BlobStateBand_Waiting},
		{"undeclared held", 21, BlobStateBand_Held},
		{"undeclared terminal", 31, BlobStateBand_Terminal},
		{"beyond the bands", 32, BlobStateBand_Unset},
		{"negative", -1, BlobStateBand_Unset},
	}
	for _, tc := range cases {
		if got := BlobStateBandOf(tc.value); got != tc.want {
			t.Errorf("%s (%d): band %v, want %v", tc.name, tc.value, got, tc.want)
		}
	}
}

// TestBlobProgressAttr: the standard attr stores amp.BlobProgress under the §4.8 name and
// mints the same UID the runtime fold does; its default cadences are declared, not baked.
func TestBlobProgressAttr(t *testing.T) {
	if Attr.BlobProgress.Text != "amp.blob.BlobProgress" {
		t.Fatalf("attr text %q, want amp.blob.BlobProgress", Attr.BlobProgress.Text)
	}
	if want := Attr.BlobAttr.With("BlobProgress").ID; Attr.BlobProgress.ID != want {
		t.Fatalf("attr UID %s != runtime fold %s", Attr.BlobProgress.ID.Base32(), want.Base32())
	}
	def, found := Registry().FindAttr(Attr.BlobProgress.ID)
	if !found {
		t.Fatal("BlobProgress is not registered")
	}
	if _, ok := def.Prototype.(*amp.BlobProgress); !ok {
		t.Fatalf("BlobProgress prototype is %T", def.Prototype)
	}
	if BlobProgressDefaultTickMs <= 0 || BlobProgressDefaultHeartbeatMs < BlobProgressDefaultTickMs || BlobProgressDefaultWantedLingerMs <= 0 {
		t.Fatalf("cadence defaults tick=%d heartbeat=%d linger=%d", BlobProgressDefaultTickMs, BlobProgressDefaultHeartbeatMs, BlobProgressDefaultWantedLingerMs)
	}
}
