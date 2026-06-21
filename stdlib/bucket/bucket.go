// Package bucket provides a steady-rate + burst token-bucket limiter — a pure
// sync+time primitive shared across amp apps (outbound email dispatch, forum
// write budgets, campaign pacing).
//
// Why token-bucket over fixed-window sleeps: provider and abuse thresholds care
// about *bursts* as well as steady-state rate.  A token-bucket lets a caller
// burn through a small reserve (a warm-up batch, a member's first few posts)
// then settle into a steady cadence; fixed-window sleeps stall the whole queue
// every period boundary.
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
// context's error if it is cancelled before a token frees up.  Callers wrap
// this around each dispatch: idle drains hold the lock for milliseconds, not
// seconds.
func (b *TokenBucket) Take(ctx context.Context) error {
	for {
		wait := b.consume()
		if wait <= 0 {
			return nil
		}
		// Wake a hair early so float64 drift doesn't oversleep.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}

// TryTake consumes one token without blocking.  Returns true when a token was
// available (the action is admitted), false when the bucket is empty (reject).
// This is the reject-style gate a request path applies before accepting a write.
func (b *TokenBucket) TryTake() bool {
	return b.consume() <= 0
}

// consume refills the bucket based on elapsed time, then attempts to consume one
// token.  Returns 0 on success, or the duration the caller should wait before a
// token frees up.
func (b *TokenBucket) consume() time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastFill).Seconds()
	b.tokens = min(b.capacity, b.tokens+elapsed*b.refillRate)
	b.lastFill = now

	if b.tokens >= 1 {
		b.tokens -= 1
		return 0
	}
	// Wait until enough refill accumulates for one token.
	deficit := 1 - b.tokens
	return time.Duration(deficit / b.refillRate * float64(time.Second))
}

// Reset re-initialises the bucket to `prefill` tokens (capped at capacity).
// Useful before a long run so the first batch can burn the burst budget without
// artificial throttling.
func (b *TokenBucket) Reset(prefill float64) {
	b.mu.Lock()
	if prefill > b.capacity {
		prefill = b.capacity
	}
	b.tokens = prefill
	b.lastFill = time.Now()
	b.mu.Unlock()
}
