//go:build race

package tag

import "sync/atomic"

func mixEntropy(in uint64) uint64 {
	e0 := atomic.LoadUint64(&gEntropy)
	e1 := (p1*in + 0xCCCCAAAACCCCAAAA) ^ (p2 * e0)
	atomic.StoreUint64(&gEntropy, e1)

	return in ^ (e1 & EntropyMask)
}
