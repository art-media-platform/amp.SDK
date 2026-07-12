package tag

import (
	"crypto/rand"
	"encoding/binary"
	"math/bits"
)

// Fractional-index rank minting for ordered channels (AD-playlists §3,
// AD-docs-channel §5.2).  A rank is a UID ordered by UID.CompareTo; render
// order is (Rank, itemID).  Ranks are minted RANDOMLY inside the open
// interval (lo, hi) — unlike the deterministic, commutative Midpoint, two
// writers inserting into the same gap draw distinct ranks, so their runs do
// not interleave.

// RankCeil is the upper fence for rank minting: append-at-end mints
// RankBetween(last, RankCeil).  The lower fence is UID{}: prepend mints
// RankBetween(UID{}, first).
var RankCeil = UID{UID_0_Max, UID_1_Max}

// RankRebalanceGap is the adjacent-rank gap below which the enclosing channel
// should rebalance (rewrite its ranks via RanksAcross) — random minting
// inside a tiny gap exhausts it within ~log2(gap) inserts.
var RankRebalanceGap = UID{0, 1 << 16}

// RankBetween returns a uniformly random UID strictly inside (lo, hi).
// ok is false when the interval has no interior point (hi <= lo+1) — the
// caller's cue to rebalance the surrounding ranks.
func RankBetween(lo, hi UID) (rank UID, ok bool) {
	if hi.CompareTo(lo) <= 0 {
		return UID{}, false
	}
	gap := hi.Subtract(lo)
	if gap[0] == 0 && gap[1] <= 1 {
		return UID{}, false
	}
	interior := gap
	interior.Decrement()
	rank = lo.Add(UID{0, 1}).Add(randBelow(interior))
	return rank, true
}

// RanksAcross partitions (lo, hi) into count consecutive slots and draws one
// rank randomly inside each — a bulk insert computes its slots up front
// instead of halving one gap repeatedly, which collapses after log2(gap)
// iterations.  Returned ranks are strictly increasing and strictly inside
// (lo, hi).  ok is false when the gap cannot hold count fenced slots
// (gap < count+1).
func RanksAcross(lo, hi UID, count int) (ranks []UID, ok bool) {
	if count <= 0 {
		return nil, count == 0
	}
	if hi.CompareTo(lo) <= 0 {
		return nil, false
	}
	gap := hi.Subtract(lo)
	slots := uint64(count) + 1
	if gap[0] == 0 && gap[1] < slots {
		return nil, false
	}
	stride := divScalar(gap, slots)
	ranks = make([]UID, count)
	for i := range ranks {
		floor := lo.Add(mulScalar(stride, uint64(i+1)))
		ranks[i] = floor.Add(randBelow(stride))
	}
	return ranks, true
}

// randBelow returns a uniformly random UID in [0, bound) via masked rejection
// sampling (crypto/rand, the NewID pattern); zero when bound is zero.
func randBelow(bound UID) UID {
	if bound.IsNil() {
		return UID{}
	}
	boundBits := 128 - leadingZeros128(bound)
	for {
		var seed [16]byte
		rand.Read(seed[:])
		draw := UID{
			binary.LittleEndian.Uint64(seed[:8]),
			binary.LittleEndian.Uint64(seed[8:]),
		}
		// Mask to the bound's bit length: rejection probability < 1/2.
		if boundBits <= 64 {
			draw[0] = 0
			if boundBits < 64 {
				draw[1] &= (uint64(1) << boundBits) - 1
			}
		} else if boundBits < 128 {
			draw[0] &= (uint64(1) << (boundBits - 64)) - 1
		}
		if draw.CompareTo(bound) < 0 {
			return draw
		}
	}
}

func leadingZeros128(id UID) int {
	if id[0] != 0 {
		return bits.LeadingZeros64(id[0])
	}
	return 64 + bits.LeadingZeros64(id[1])
}

// mulScalar multiplies a UID by a scalar (mod 2^128).
func mulScalar(id UID, scalar uint64) UID {
	hi, lo := bits.Mul64(id[1], scalar)
	return UID{id[0]*scalar + hi, lo}
}

// divScalar divides a UID by a nonzero scalar.
func divScalar(id UID, scalar uint64) UID {
	quoHi := id[0] / scalar
	rem := id[0] % scalar
	quoLo, _ := bits.Div64(rem, id[1], scalar)
	return UID{quoHi, quoLo}
}
