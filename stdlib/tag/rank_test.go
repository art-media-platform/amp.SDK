package tag_test

import (
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

func TestRankBetweenTable(t *testing.T) {
	tests := []struct {
		name   string
		lo, hi tag.UID
		wantOK bool
	}{
		{"equal", tag.UID{5, 5}, tag.UID{5, 5}, false},
		{"swapped", tag.UID{5, 10}, tag.UID{5, 2}, false},
		{"gap of one — no interior", tag.UID{0, 7}, tag.UID{0, 8}, false},
		{"gap of two — one interior point", tag.UID{0, 7}, tag.UID{0, 9}, true},
		{"word boundary", tag.UID{0, 0xFFFFFFFFFFFFFFFF}, tag.UID{1, 1}, true},
		{"full fence span", tag.UID{}, tag.RankCeil, true},
		{"wide gap", tag.UID{0, 1 << 20}, tag.UID{0, 1 << 40}, true},
	}
	for _, tt := range tests {
		rank, ok := tag.RankBetween(tt.lo, tt.hi)
		if ok != tt.wantOK {
			t.Errorf("%s: ok = %v, want %v", tt.name, ok, tt.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if rank.CompareTo(tt.lo) <= 0 || rank.CompareTo(tt.hi) >= 0 {
			t.Errorf("%s: rank %v not strictly inside (%v, %v)", tt.name, rank, tt.lo, tt.hi)
		}
	}

	// The gap-of-two case has exactly one interior point — deterministic.
	rank, _ := tag.RankBetween(tag.UID{0, 7}, tag.UID{0, 9})
	if rank != (tag.UID{0, 8}) {
		t.Errorf("single interior point: got %v, want {0,8}", rank)
	}
}

func TestRankBetweenInterior(t *testing.T) {
	for range 2000 {
		lo, hi := tag.NewID(), tag.NewID()
		if hi.CompareTo(lo) < 0 {
			lo, hi = hi, lo
		}
		rank, ok := tag.RankBetween(lo, hi)
		if !ok {
			continue // adjacent or equal draws — legitimately no interior
		}
		if rank.CompareTo(lo) <= 0 || rank.CompareTo(hi) >= 0 {
			t.Fatalf("rank %v escapes (%v, %v)", rank, lo, hi)
		}
	}
}

func TestRankBetweenNonDeterministic(t *testing.T) {
	// Two draws into the same wide gap must differ (w.h.p. ~2^-90) — the
	// no-interleave property Midpoint lacks.
	lo, hi := tag.UID{}, tag.RankCeil
	first, _ := tag.RankBetween(lo, hi)
	for range 4 {
		next, _ := tag.RankBetween(lo, hi)
		if next != first {
			return
		}
	}
	t.Error("five identical draws from a 2^128 gap — RankBetween is not random")
}

func TestRanksAcrossMonotone(t *testing.T) {
	// 2^26 = the tightest adjacent-NowID gap measured in AD-playlists §3;
	// the partition table there: 100/1000/10000 slots all fit.
	lo := tag.UID{0, 1 << 30}
	hi := lo.Add(tag.UID{0, 1 << 26})
	for _, count := range []int{1, 100, 1000, 10000} {
		ranks, ok := tag.RanksAcross(lo, hi, count)
		if !ok || len(ranks) != count {
			t.Fatalf("count %d: ok=%v len=%d", count, ok, len(ranks))
		}
		prev := lo
		for i, rank := range ranks {
			if rank.CompareTo(prev) <= 0 {
				t.Fatalf("count %d: rank[%d] %v <= predecessor %v", count, i, rank, prev)
			}
			prev = rank
		}
		if prev.CompareTo(hi) >= 0 {
			t.Fatalf("count %d: last rank %v >= hi %v", count, prev, hi)
		}
	}
}

func TestRanksAcrossBounds(t *testing.T) {
	lo := tag.UID{0, 100}

	if ranks, ok := tag.RanksAcross(lo, lo.Add(tag.UID{0, 8}), 7); !ok || len(ranks) != 7 {
		t.Errorf("gap 8, count 7 (gap == count+1, stride 1): ok=%v len=%d", ok, len(ranks))
	}
	if _, ok := tag.RanksAcross(lo, lo.Add(tag.UID{0, 7}), 7); ok {
		t.Error("gap 7, count 7: want ok=false")
	}
	if _, ok := tag.RanksAcross(lo.Add(tag.UID{0, 8}), lo, 3); ok {
		t.Error("swapped fences: want ok=false")
	}
	if ranks, ok := tag.RanksAcross(lo, lo.Add(tag.UID{0, 8}), 0); !ok || len(ranks) != 0 {
		t.Error("count 0: want empty, ok=true")
	}
	if _, ok := tag.RanksAcross(lo, lo.Add(tag.UID{0, 8}), -1); ok {
		t.Error("negative count: want ok=false")
	}

	// Full fence-to-fence bulk partition stays inside legal rank space.
	ranks, ok := tag.RanksAcross(tag.UID{}, tag.RankCeil, 1000)
	if !ok {
		t.Fatal("fence-to-fence partition failed")
	}
	if ranks[0].IsNil() || ranks[len(ranks)-1].CompareTo(tag.RankCeil) >= 0 {
		t.Error("fence-to-fence ranks escape (UID{}, RankCeil)")
	}
}

func TestRankFuzzInterior(t *testing.T) {
	for range 500 {
		lo, hi := tag.NewID(), tag.NewID()
		if hi.CompareTo(lo) < 0 {
			lo, hi = hi, lo
		}
		ranks, ok := tag.RanksAcross(lo, hi, 8)
		if !ok {
			continue
		}
		prev := lo
		for _, rank := range ranks {
			if rank.CompareTo(prev) <= 0 || rank.CompareTo(hi) >= 0 {
				t.Fatalf("ranks escape or collide in (%v, %v)", lo, hi)
			}
			prev = rank
		}
	}
}
