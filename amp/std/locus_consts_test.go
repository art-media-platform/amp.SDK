package std_test

import (
	"testing"

	"github.com/art-media-platform/amp.SDK/amp/std"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// The tag package mirrors the Locus constants from amp.std.consts.sdl (an
// import cycle bars it from reading them).  This guard fails the build the
// moment the two copies drift.
func TestLocusConstsMirrorStd(t *testing.T) {
	base := tag.UID{0xAAAA, 0xBBBBBBBBBBBBBB40} // low 6 bits zero
	if got := base.WithCell(int(std.LocusSpan - 1)).LocusCell(); got != int(std.LocusSpan-1) {
		t.Fatalf("tag locus mask disagrees with std.LocusSpan: cell %d != %d", got, std.LocusSpan-1)
	}
	if got := base.WithCell(int(std.LocusSpan)).LocusCell(); got != 0 {
		t.Fatalf("tag locus span exceeds std.LocusSpan: cell %d != 0", got)
	}
	if std.LocusMask != uint64(std.LocusSpan-1) {
		t.Fatalf("std.LocusMask %#x != LocusSpan-1 %#x", std.LocusMask, uint64(std.LocusSpan-1))
	}
}
