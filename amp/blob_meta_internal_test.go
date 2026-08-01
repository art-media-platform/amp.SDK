package amp

import (
	"bytes"
	"testing"
)

// The lane counter arithmetic carries the exponent boundaries that byte-level
// tests cannot reach in budget (the first lane death sits at 32 GiB): emit
// cadence, the kill threshold at exactly target<<exp vs one byte over, and
// pick-consistency with ChooseBlobChunkSizeLog2.
func TestBlobLaneCounters(t *testing.T) {
	if blobGrainsPerChunk(BlobChunkSizeLog2Min) != 1<<(BlobChunkSizeLog2Min-BlobGrainSizeLog2) {
		t.Error("grains-per-chunk cadence wrong at the min exponent")
	}
	if blobGrainsPerChunk(BlobChunkSizeLog2Max) != 1<<(BlobChunkSizeLog2Max-BlobGrainSizeLog2) {
		t.Error("grains-per-chunk cadence wrong at the max exponent")
	}
	for exp := uint32(BlobChunkSizeLog2Min); exp < BlobChunkSizeLog2Max; exp++ {
		atCap := int64(BlobMetaTargetChunks) << exp
		if blobLaneDead(atCap, exp) {
			t.Errorf("lane %d dead at exactly target<<exp — off-by-one kills the winning lane", exp)
		}
		if !blobLaneDead(atCap+1, exp) {
			t.Errorf("lane %d alive one byte past target<<exp", exp)
		}
	}
	for _, storedLen := range []int64{1, 4097, 2 << 30, 32 << 30, (32 << 30) + 1, 2 << 40, BlobLenMax} {
		exp := ChooseBlobChunkSizeLog2(storedLen)
		if exp < BlobChunkSizeLog2Max && blobLaneDead(storedLen, exp) {
			t.Errorf("chosen exponent %d for %d bytes is a dead lane — pick and prune disagree", exp, storedLen)
		}
	}
}

// Forced-spill parity: the lane path (giant-blob posture) must mint the
// byte-identical meta the buffered path mints — proven at small scale by
// spilling early, including before the first byte.
func TestBlobMetaBuilder_SpillParity(t *testing.T) {
	blob := make([]byte, 10<<20)
	for ii := range blob {
		blob[ii] = byte(ii*29+3) ^ byte(ii>>10)
	}
	mint := func(spillAt int) []byte {
		builder, err := NewBlobMetaBuilder(0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := builder.Write(blob[:spillAt]); err != nil {
			t.Fatal(err)
		}
		builder.spill()
		if _, err := builder.Write(blob[spillAt:]); err != nil {
			t.Fatal(err)
		}
		meta, err := builder.Finish()
		if err != nil {
			t.Fatal(err)
		}
		if meta.ChunkSizeLog2 != ChooseBlobChunkSizeLog2(int64(len(blob))) {
			t.Fatalf("lane pick %d != ChooseBlobChunkSizeLog2", meta.ChunkSizeLog2)
		}
		return meta.CanonicalBytes()
	}
	buffered := func() []byte {
		builder, err := NewBlobMetaBuilder(0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := builder.Write(blob); err != nil {
			t.Fatal(err)
		}
		meta, err := builder.Finish()
		if err != nil {
			t.Fatal(err)
		}
		return meta.CanonicalBytes()
	}()
	for _, spillAt := range []int{0, 5000, 3 << 20, len(blob)} {
		if !bytes.Equal(mint(spillAt), buffered) {
			t.Errorf("spill at %d bytes: lane path diverges from the buffered path", spillAt)
		}
	}
}
