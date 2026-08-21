// Package bucket provides a steady-rate + burst token-bucket rate limiter — a
// pure sync+time primitive with a blocking gate (Take) and non-blocking
// reject gates (TryTake, TryTakeN).
//
// A token bucket bounds bursts as well as steady-state rate: a caller may burn
// a small reserve, then settles into the refill cadence — where fixed-window
// sleeps would stall the whole queue at every period boundary.
package bucket

import (
	"context"
	"sync"
	"time"
)

// TokenBucket is a steady-rate + burst limiter.  All methods are threadsafe.
type TokenBucket struct {
	capacity   float64 // max tokens (burst budget)
	refillRate float64 // tokens/sec

	mu       sync.Mutex
	tokens   float64
	lastFill time.Time
}

// NewTokenBucket configures a bucket with burst `capacity` and steady-state
// refill `refillRatePerSec` (tokens per second).  The bucket starts full, so
// the first `capacity` takes succeed before throttling begins.
func NewTokenBucket(capacity, refillRatePerSec float64) *TokenBucket {
	return &TokenBucket{
		capacity:   capacity,
		refillRate: refillRatePerSec,
		tokens:     capacity,
		lastFill:   time.Now(),
	}
}

// Take blocks until one token is available, then consumes it.  Returns the
// context's error if it is cancelled before a token frees up.
func (b *TokenBucket) Take(ctx context.Context) error {
	for {
		admitted, wait := b.TryTakeN(1)
		if admitted {
			return nil
		}
		// Sleep out the deficit, then re-check; another taker may have raced us.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}

// TryTake consumes one token without blocking.  Returns true when a token was
// available (the action is admitted), false when the bucket is empty (reject).
func (b *TokenBucket) TryTake() bool {
	admitted, _ := b.TryTakeN(1)
	return admitted
}

// TryTakeN consumes `count` tokens atomically without blocking: either all
// `count` are taken (admitted, zero wait) or none are (rejected, with the
// duration until the full count refills — the Retry-After a refusal carries).
// A count above capacity can never be admitted; size the burst so the largest
// legitimate take fits.
func (b *TokenBucket) TryTakeN(count float64) (bool, time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastFill).Seconds()
	b.tokens = min(b.capacity, b.tokens+elapsed*b.refillRate)
	b.lastFill = now

	if b.tokens >= count {
		b.tokens -= count
		return true, 0
	}
	// Wait until enough refill accumulates for the full count.
	deficit := count - b.tokens
	return false, time.Duration(deficit / b.refillRate * float64(time.Second))
}

// Reset re-initializes the bucket to `prefill` tokens (capped at capacity).
func (b *TokenBucket) Reset(prefill float64) {
	b.mu.Lock()
	if prefill > b.capacity {
		prefill = b.capacity
	}
	b.tokens = prefill
	b.lastFill = time.Now()
	b.mu.Unlock()
}
