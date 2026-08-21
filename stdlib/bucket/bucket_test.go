package bucket

import (
	"testing"
	"time"
)

// Refill rate slow enough that no meaningful refill occurs mid-test.
const frozenRate = 0.0001

func TestTryTakeN_AllOrNothing(t *testing.T) {
	bkt := NewTokenBucket(10, frozenRate)

	if admitted, _ := bkt.TryTakeN(8); !admitted {
		t.Fatal("full bucket refused a take within capacity")
	}
	if admitted, wait := bkt.TryTakeN(8); admitted {
		t.Fatal("admitted a take beyond remaining tokens")
	} else if wait <= 0 {
		t.Fatal("refusal carried no retry-after wait")
	}
	// The refusal above must not have consumed the 2 remaining tokens.
	if admitted, _ := bkt.TryTakeN(2); !admitted {
		t.Fatal("refusal consumed partial tokens — TryTakeN must be atomic")
	}
}

func TestTryTakeN_RetryAfterTracksDeficit(t *testing.T) {
	bkt := NewTokenBucket(10, 1) // 1 token/sec
	bkt.Reset(0)

	admitted, wait := bkt.TryTakeN(5)
	if admitted {
		t.Fatal("empty bucket admitted a take")
	}
	// Deficit of ~5 tokens at 1/sec ⇒ ~5s (refill during the call only shrinks it).
	if wait < 4*time.Second || wait > 5*time.Second+100*time.Millisecond {
		t.Fatalf("retry-after %v, want ~5s for a 5-token deficit at 1/sec", wait)
	}
}

func TestTryTakeN_CountAboveCapacityRefused(t *testing.T) {
	bkt := NewTokenBucket(4, frozenRate)
	if admitted, wait := bkt.TryTakeN(5); admitted {
		t.Fatal("admitted a count above capacity")
	} else if wait <= 0 {
		t.Fatal("over-capacity refusal carried no wait")
	}
	if admitted, _ := bkt.TryTakeN(4); !admitted {
		t.Fatal("over-capacity refusal consumed tokens")
	}
}

func TestTryTake_BurstThenRefuse(t *testing.T) {
	bkt := NewTokenBucket(2, frozenRate)
	for range 2 {
		if !bkt.TryTake() {
			t.Fatal("burst capacity not honored")
		}
	}
	if bkt.TryTake() {
		t.Fatal("empty bucket admitted a take")
	}
}
