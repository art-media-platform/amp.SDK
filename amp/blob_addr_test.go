package amp

// blob_addr_test.go — golden-value fixture pinning the meta-chunk address
// arithmetic (SD-planet-storage §13.10).  Every expected value is a literal
// derived independently of the helpers, so a drifted shift or an off-by-one
// fails here.  (The sealed-frame space's twin fixture lives planet-side in
// vault.blobaddr_test.go — the seal is vault-private.)

import (
	"testing"
)

func TestBlobChunkArithmeticGolden(t *testing.T) {
	offsetCases := []struct {
		chunkIndex uint64
		log2       uint32
		offset     int64
	}{
		{0, 20, 0},
		{3, 20, 3145728},
		{5, 24, 83886080},
		{2, 30, 2147483648},
	}
	for _, c := range offsetCases {
		if got := BlobChunkOffset(c.chunkIndex, c.log2); got != c.offset {
			t.Errorf("BlobChunkOffset(%d, %d) = %d, want %d", c.chunkIndex, c.log2, got, c.offset)
		}
	}

	indexCases := []struct {
		position int64
		log2     uint32
		index    uint64
	}{
		{0, 20, 0},
		{3145727, 20, 2},
		{3145728, 20, 3},
		{83886079, 24, 4},
	}
	for _, c := range indexCases {
		if got := BlobChunkIndex(c.position, c.log2); got != c.index {
			t.Errorf("BlobChunkIndex(%d, %d) = %d, want %d", c.position, c.log2, got, c.index)
		}
	}

	remainingCases := []struct {
		position  int64
		log2      uint32
		remaining int64
	}{
		{0, 20, 1048576},
		{1, 20, 1048575},
		{1048575, 20, 1},
		{1048576, 20, 1048576},
		{16777223, 24, 16777209},
	}
	for _, c := range remainingCases {
		if got := BlobChunkRemaining(c.position, c.log2); got != c.remaining {
			t.Errorf("BlobChunkRemaining(%d, %d) = %d, want %d", c.position, c.log2, got, c.remaining)
		}
	}

	alignedCases := []struct {
		position int64
		log2     uint32
		aligned  bool
	}{
		{0, 20, true},
		{1048576, 20, true},
		{999, 20, false},
		{2097152, 20, true},
		{16777216, 24, true},
		{16777217, 24, false},
	}
	for _, c := range alignedCases {
		if got := BlobChunkAligned(c.position, c.log2); got != c.aligned {
			t.Errorf("BlobChunkAligned(%d, %d) = %v, want %v", c.position, c.log2, got, c.aligned)
		}
	}
}

func TestBlobWireAddressGolden(t *testing.T) {
	cases := []struct {
		position      int64
		log2          uint32
		chunkIndex    uint64
		offsetInChunk uint64
	}{
		{12345, 0, 0, 12345}, // no meta: one implicit chunk
		{0, 20, 0, 0},
		{5242957, 20, 5, 77},
		{1048576, 20, 1, 0},
		{83886081, 24, 5, 1},
	}
	for _, c := range cases {
		chunkIndex, offsetInChunk := BlobWireAddress(c.position, c.log2)
		if chunkIndex != c.chunkIndex || offsetInChunk != c.offsetInChunk {
			t.Errorf("BlobWireAddress(%d, %d) = (%d, %d), want (%d, %d)",
				c.position, c.log2, chunkIndex, offsetInChunk, c.chunkIndex, c.offsetInChunk)
		}
	}
}

func TestBlobPullSpanGolden(t *testing.T) {
	cases := []struct {
		name        string
		blobLen     int64
		log2        uint32
		chunkBegin  uint64
		chunkCount  uint64
		startOffset int64
		spanLen     int64
		ok          bool
	}{
		{"mid-span capped", 10485760, 20, 2, 3, 2097152, 3145728, true},
		{"through end", 10485760, 20, 2, 0, 2097152, 8388608, true},
		{"count past end clamps", 10485760, 20, 9, 8, 9437184, 1048576, true},
		{"short final chunk", 2097153, 20, 2, 0, 2097152, 1, true},
		{"begin at end rejects", 10485760, 20, 10, 0, 0, 0, false},
		{"exponent 63 rejects", 10485760, 63, 0, 0, 0, 0, false},
		{"begin past BlobLenMax>>log2 rejects", 10485760, 20, (1 << 30) + 1, 0, 0, 0, false},
		{"no meta whole blob", 999, 0, 0, 0, 0, 999, true},
		{"no meta count caps bytes", 999, 0, 0, 5, 0, 5, true},
		{"no meta nonzero begin rejects", 999, 0, 1, 0, 0, 0, false},
		{"zero-length blob", 0, 0, 0, 0, 0, 0, true},
	}
	for _, c := range cases {
		startOffset, spanLen, ok := BlobPullSpan(c.blobLen, c.log2, c.chunkBegin, c.chunkCount)
		if startOffset != c.startOffset || spanLen != c.spanLen || ok != c.ok {
			t.Errorf("%s: BlobPullSpan(%d, %d, %d, %d) = (%d, %d, %v), want (%d, %d, %v)",
				c.name, c.blobLen, c.log2, c.chunkBegin, c.chunkCount,
				startOffset, spanLen, ok, c.startOffset, c.spanLen, c.ok)
		}
	}
}
