package amp_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"google.golang.org/protobuf/proto"
)

// Deterministic synthetic STORED bytes: 2 full 1 MiB chunks + a 3-byte tail
// (3 meta chunks at 2^20).  The pattern is fixed so the goldens below pin the
// canonical encoding and root commitment across refactors and languages.
const goldenBlobLen = (2 << 20) + 3

func goldenBlobBytes() []byte {
	blob := make([]byte, goldenBlobLen)
	for i := range blob {
		blob[i] = byte(i*131+7) ^ byte(i>>16)
	}
	return blob
}

// buildMeta mints a meta through the one production mint site.
func buildMeta(t *testing.T, blob []byte) *amp.BlobMeta {
	t.Helper()
	builder, err := amp.NewBlobMetaBuilder(0)
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
	return meta
}

func goldenBlobRef(t *testing.T, blob []byte) (*amp.BlobRef, *amp.BlobMeta) {
	t.Helper()
	meta := buildMeta(t, blob)
	ref := &amp.BlobRef{
		BlobTag: amp.TagFromUID(tag.UID{0xBB, 0xEE}),
	}
	ref.BlobTag.I = int64(len(blob))
	if err := ref.SetBlobMeta(meta, ref.HashKitID); err != nil {
		t.Fatal(err)
	}
	return ref, meta
}

// GOLDEN (mint-once; identity is bytes): the canonical meta encoding and its
// root commitment for goldenBlobBytes under the default HashKit at 2^20.
// A diff here is a wire break — the canonical encoding or the digest moved.
const (
	goldenMetaCanonicalHex = "081410838080011a6005e983d638e93cdf1b081cb754587b3515648778839403a8c71232c5d8a7dfa42c36cd8421ddd77ff916fc44ad42ac666d689c4a729b1dd92d64c90299d443f8847bc2c76f02b13a3e0b8c9b974704c4e65ca87bfa289b94731541b33f086b3c"
	goldenMetaRootHex      = "65260dacc5ea2c89483a853f6042d40b"
)

func TestBlobMeta_Golden(t *testing.T) {
	_, meta := goldenBlobRef(t, goldenBlobBytes())
	canonHex := hex.EncodeToString(meta.CanonicalBytes())
	if canonHex != goldenMetaCanonicalHex {
		t.Errorf("canonical meta encoding drifted from golden:\n got: %s\nwant: %s", canonHex, goldenMetaCanonicalHex)
	}
	root, err := meta.RootUID(0)
	if err != nil {
		t.Fatal(err)
	}
	rootBytes := root.AppendTo(nil)
	if hex.EncodeToString(rootBytes) != goldenMetaRootHex {
		t.Errorf("meta root drifted from golden:\n got: %s\nwant: %s", hex.EncodeToString(rootBytes), goldenMetaRootHex)
	}
}

// The canonical encoding is standard proto3 wire form, fields in order — what
// any language's marshaler emits for this message shape.
func TestBlobMeta_CanonicalMatchesProto(t *testing.T) {
	_, meta := goldenBlobRef(t, goldenBlobBytes())
	protoBytes, err := proto.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(protoBytes, meta.CanonicalBytes()) {
		t.Errorf("CanonicalBytes != proto.Marshal:\n canon: %x\n proto: %x", meta.CanonicalBytes(), protoBytes)
	}
}

// Receiver-side round trip: the meta arrives as wire bytes, is verified
// against the ref's signed commitment, then gates every chunk.
func TestBlobMeta_ReceiverVerifies(t *testing.T) {
	blob := goldenBlobBytes()
	ref, builtMeta := goldenBlobRef(t, blob)

	wireBytes, err := proto.Marshal(builtMeta)
	if err != nil {
		t.Fatal(err)
	}
	received := &amp.BlobMeta{}
	if err := proto.Unmarshal(wireBytes, received); err != nil {
		t.Fatal(err)
	}
	if err := ref.VerifyBlobMeta(received); err != nil {
		t.Fatalf("valid meta failed receiver verification: %v", err)
	}
	if received.NumChunks() != 3 {
		t.Fatalf("expected 3 chunks, got %d", received.NumChunks())
	}
	for chunkIndex := uint64(0); chunkIndex < received.NumChunks(); chunkIndex++ {
		offset, length := received.ChunkSpan(chunkIndex)
		if err := received.VerifyChunk(chunkIndex, blob[offset:offset+length], ref.HashKitID); err != nil {
			t.Fatalf("chunk %d failed verification: %v", chunkIndex, err)
		}
	}
	if _, tailLen := received.ChunkSpan(2); tailLen != 3 {
		t.Fatalf("tail chunk length %d, want 3", tailLen)
	}
}

// Receiver-side rejection: a corrupted or mismatched meta (or chunk) must
// fail loudly at the receiver — never only at the encoder.
func TestBlobMeta_ReceiverRejects(t *testing.T) {
	blob := goldenBlobBytes()
	ref, meta := goldenBlobRef(t, blob)

	corruptChunk := append([]byte{}, blob[:1<<20]...)
	corruptChunk[12345] ^= 1
	if err := meta.VerifyChunk(0, corruptChunk, ref.HashKitID); err == nil {
		t.Error("corrupted chunk content passed meta verification")
	}
	if err := meta.VerifyChunk(0, blob[:100], ref.HashKitID); err == nil {
		t.Error("short chunk passed meta verification")
	}
	if err := meta.VerifyChunk(9, blob[:3], ref.HashKitID); err == nil {
		t.Error("out-of-range chunk index passed meta verification")
	}

	tampered := proto.Clone(meta).(*amp.BlobMeta)
	tampered.ChunkHashes[40] ^= 1
	if err := ref.VerifyBlobMeta(tampered); err == nil {
		t.Error("corrupted meta hash bytes passed root verification")
	}

	tampered = proto.Clone(meta).(*amp.BlobMeta)
	tampered.TotalLen += 1 << 20
	tampered.ChunkHashes = append(tampered.ChunkHashes, make([]byte, amp.BlobMetaHashSize)...)
	if err := ref.VerifyBlobMeta(tampered); err == nil {
		t.Error("meta with forged TotalLen passed verification")
	}

	tampered = proto.Clone(meta).(*amp.BlobMeta)
	tampered.ChunkSizeLog2 = 21
	if err := ref.VerifyBlobMeta(tampered); err == nil {
		t.Error("meta with mismatched ChunkSizeLog2 passed verification")
	}

	tampered = proto.Clone(meta).(*amp.BlobMeta)
	tampered.ChunkHashes = tampered.ChunkHashes[:2*amp.BlobMetaHashSize]
	if err := ref.VerifyBlobMeta(tampered); err == nil {
		t.Error("truncated meta passed verification")
	}

	// A mismatched meta: internally consistent (self-rooted) but for other
	// content — the ref's commitment must reject it.
	otherBlob := goldenBlobBytes()
	otherBlob[0] ^= 1
	otherMeta := buildMeta(t, otherBlob)
	if err := ref.VerifyBlobMeta(otherMeta); err == nil {
		t.Error("another blob's meta passed this ref's commitment")
	}

	bare := &amp.BlobRef{}
	if err := bare.VerifyBlobMeta(meta); err == nil {
		t.Error("ref with no commitment accepted a meta")
	}
}

func TestChooseBlobChunkSizeLog2(t *testing.T) {
	cases := []struct {
		storedLen int64
		wantExp   uint32
	}{
		{150 << 20, 20},      // 150 MB show scale → 1 MiB, 150 chunks
		{2 << 30, 20},        // 2 GiB → 2048 chunks at 1 MiB
		{32 << 30, 20},       // 32 GiB → exactly 32768 chunks at 1 MiB
		{(32 << 30) + 1, 21}, // one byte over the ceiling → 2 MiB
		{2 << 40, 26},        // 2 TiB → 64 MiB, exactly 32768 chunks
		{100 << 40, 30},      // beyond target at max exponent — capped
		{3, 20},              // tiny → floor (grain floor decides no-meta)
	}
	for _, one := range cases {
		if got := amp.ChooseBlobChunkSizeLog2(one.storedLen); got != one.wantExp {
			t.Errorf("ChooseBlobChunkSizeLog2(%d) = %d, want %d", one.storedLen, got, one.wantExp)
		}
	}
	for _, one := range cases {
		count := amp.BlobChunkCount(one.storedLen, one.wantExp)
		if count > amp.BlobMetaTargetChunks && one.wantExp < amp.BlobChunkSizeLog2Max {
			t.Errorf("storedLen %d: %d chunks exceeds target below the cap", one.storedLen, count)
		}
	}
}

// patternBytes fills a deterministic length-dependent pattern.
func patternBytes(byteLen int) []byte {
	blob := make([]byte, byteLen)
	for ii := range blob {
		blob[ii] = byte(ii*167+13) ^ byte(ii>>12) ^ byte(byteLen)
	}
	return blob
}

// naiveMeta is the test-local reference: the §13.10 two-level definition
// computed the obvious way, deliberately independent of the builder (the
// compose tag 0x01 is restated here on purpose — a tag drift must fail).
func naiveMeta(t *testing.T, blob []byte) *amp.BlobMeta {
	t.Helper()
	grainSize := 1 << amp.BlobGrainSizeLog2
	if len(blob) <= grainSize {
		return nil
	}
	kit, err := safe.NewHashKit(0)
	if err != nil {
		t.Fatal(err)
	}
	exp := amp.ChooseBlobChunkSizeLog2(int64(len(blob)))
	chunkSize := 1 << int(exp)
	meta := &amp.BlobMeta{
		ChunkSizeLog2: exp,
		TotalLen:      uint64(len(blob)),
	}
	for begin := 0; begin < len(blob); begin += chunkSize {
		chunkEnd := min(begin+chunkSize, len(blob))
		var run []byte
		for grainBegin := begin; grainBegin < chunkEnd; grainBegin += grainSize {
			grainEnd := min(grainBegin+grainSize, chunkEnd)
			kit.Hasher.Reset()
			kit.Hasher.Write(blob[grainBegin:grainEnd])
			run = append(run, kit.Hasher.Sum(nil)[:amp.BlobMetaHashSize]...)
		}
		kit.Hasher.Reset()
		kit.Hasher.Write([]byte{0x01})
		kit.Hasher.Write(run)
		meta.ChunkHashes = append(meta.ChunkHashes, kit.Hasher.Sum(nil)[:amp.BlobMetaHashSize]...)
	}
	return meta
}

// Builder-vs-reference parity across the shape space: grain floor, one-entry
// sub-MiB metas, partial final grain, partial and aligned final chunks.
func TestBlobMetaBuilder_ParityWithReference(t *testing.T) {
	sizes := []int{
		1 << amp.BlobGrainSizeLog2,       // exactly one grain — no meta
		(1 << amp.BlobGrainSizeLog2) + 1, // one byte over — one-entry meta
		100_000,                          // sub-MiB, partial final grain
		1 << 20,                          // exactly one chunk, full run
		(1 << 20) + 1,                    // two chunks, one-grain second run
		(5 << 20) / 2,                    // 2.5 MiB
		(4 << 20) + 3,                    // partial final grain AND chunk
		8 << 20,                          // aligned final chunk
	}
	for _, byteLen := range sizes {
		blob := patternBytes(byteLen)
		built := buildMeta(t, blob)
		want := naiveMeta(t, blob)
		if (built == nil) != (want == nil) {
			t.Fatalf("len %d: builder meta nil=%v, reference nil=%v", byteLen, built == nil, want == nil)
		}
		if built == nil {
			continue
		}
		if !bytes.Equal(built.CanonicalBytes(), want.CanonicalBytes()) {
			t.Errorf("len %d: builder meta diverges from the naive reference", byteLen)
		}
		for chunkIndex := uint64(0); chunkIndex < built.NumChunks(); chunkIndex++ {
			offset, length := built.ChunkSpan(chunkIndex)
			if err := built.VerifyChunk(chunkIndex, blob[offset:offset+length], 0); err != nil {
				t.Errorf("len %d chunk %d: %v", byteLen, chunkIndex, err)
			}
		}
	}
}

// The flat definition is dead and the compose tag is live: a meta entry
// matches neither H(chunk bytes) nor the untagged H(run).
func TestBlobMetaEntry_NotFlatAndTagged(t *testing.T) {
	blob := patternBytes(2 << 20)
	meta := buildMeta(t, blob)
	entry := meta.ChunkHash(0)
	chunk := blob[:1<<20]

	kit, err := safe.NewHashKit(0)
	if err != nil {
		t.Fatal(err)
	}
	kit.Hasher.Reset()
	kit.Hasher.Write(chunk)
	if bytes.Equal(kit.Hasher.Sum(nil)[:amp.BlobMetaHashSize], entry) {
		t.Error("meta entry equals the flat chunk hash — the two-level definition is not in effect")
	}

	run, err := amp.BlobGrainRun(0, chunk)
	if err != nil {
		t.Fatal(err)
	}
	kit.Hasher.Reset()
	kit.Hasher.Write(run)
	if bytes.Equal(kit.Hasher.Sum(nil)[:amp.BlobMetaHashSize], entry) {
		t.Error("meta entry equals the UNTAGGED run hash — the compose tag is not in effect")
	}
	if err := amp.VerifyGrainRun(0, entry, run); err != nil {
		t.Errorf("derived run failed against its own entry: %v", err)
	}
}

// Identical bytes in any write segmentation yield byte-identical metas.
func TestBlobMetaBuilder_WritePatternIndependence(t *testing.T) {
	blob := patternBytes((5 << 20) / 2)
	want := buildMeta(t, blob).CanonicalBytes()
	for _, sliceLen := range []int{1, 4096, (1 << 20) + 1} {
		builder, err := amp.NewBlobMetaBuilder(0)
		if err != nil {
			t.Fatal(err)
		}
		for begin := 0; begin < len(blob); begin += sliceLen {
			if _, err := builder.Write(blob[begin:min(begin+sliceLen, len(blob))]); err != nil {
				t.Fatal(err)
			}
		}
		meta, err := builder.Finish()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(meta.CanonicalBytes(), want) {
			t.Errorf("write slice %d: meta diverges", sliceLen)
		}
	}
}

// Grain-run primitives: the narrow-link gate accepts a derived run and
// rejects a corrupted, truncated, or empty one.
func TestBlobGrainRun_Primitives(t *testing.T) {
	blob := patternBytes((1 << 20) + 5000)
	meta := buildMeta(t, blob)
	chunk := blob[:1<<20]
	entry := meta.ChunkHash(0)

	run, err := amp.BlobGrainRun(0, chunk)
	if err != nil {
		t.Fatal(err)
	}
	if len(run) != 256*amp.BlobMetaHashSize {
		t.Fatalf("full-chunk run is %d bytes, want %d", len(run), 256*amp.BlobMetaHashSize)
	}
	if err := amp.VerifyGrainRun(0, entry, run); err != nil {
		t.Fatalf("valid run rejected: %v", err)
	}
	corrupt := append([]byte{}, run...)
	corrupt[100] ^= 1
	if err := amp.VerifyGrainRun(0, entry, corrupt); err == nil {
		t.Error("corrupted run passed")
	}
	if err := amp.VerifyGrainRun(0, entry, run[:31]); err == nil {
		t.Error("truncated run passed")
	}
	if err := amp.VerifyGrainRun(0, entry, nil); err == nil {
		t.Error("empty run passed")
	}
}

// Sub-MiB blobs now carry a one-entry meta (§13.10 meta floor: > one grain);
// at or under one grain stays meta-free.
func TestBlobMeta_SubMiBSingleEntry(t *testing.T) {
	blob := patternBytes(100_000)
	meta := buildMeta(t, blob)
	if meta == nil {
		t.Fatal("sub-MiB multi-grain blob minted no meta")
	}
	if meta.NumChunks() != 1 || meta.ChunkSizeLog2 != amp.BlobChunkSizeLog2Min {
		t.Fatalf("got %d chunks at 2^%d, want 1 at 2^%d", meta.NumChunks(), meta.ChunkSizeLog2, amp.BlobChunkSizeLog2Min)
	}
	ref := &amp.BlobRef{
		BlobTag: amp.TagFromUID(tag.UID{0xAA, 0x11}),
	}
	ref.BlobTag.I = int64(len(blob))
	if err := ref.SetBlobMeta(meta, 0); err != nil {
		t.Fatal(err)
	}
	if err := ref.VerifyBlobMeta(meta); err != nil {
		t.Fatalf("one-entry meta failed receiver verification: %v", err)
	}
	if err := meta.VerifyChunk(0, blob, 0); err != nil {
		t.Fatalf("one-entry chunk verify: %v", err)
	}

	uid := meta.ChunkUID(0)
	entry := meta.ChunkHash(0)
	if uid.AppendTo(nil) == nil || !bytes.Equal(uid.AppendTo(nil), entry[:16]) {
		t.Error("ChunkUID is not the entry's leading 16 bytes")
	}
}
