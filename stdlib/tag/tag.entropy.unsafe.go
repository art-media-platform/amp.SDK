//go:build !race

package tag

func mixEntropy(in uint64) uint64 {
	e0 := gEntropy
	e1 := (p1*in + 0xCCCCAAAACCCCAAAA) ^ (p2 * e0)
	gEntropy = e1

	return in ^ (e1 & EntropyMask)
}
