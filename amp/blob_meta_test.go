package amp_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
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

func goldenBlobRef(t *testing.T, blob []byte) (*amp.BlobRef, *amp.BlobMeta) {
	t.Helper()
	meta, err := amp.BuildBlobMeta(bytes.NewReader(blob), int64(len(blob)), 0, amp.BlobChunkSizeLog2Min)
	if err != nil {
		t.Fatal(err)
	}
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
	goldenMetaCanonicalHex = "081410838080011a60d9e4a620c43e545fe2d81eaed81467928679db133741562c196d21b2faf1a9c3daeaa208a590528db463586b9de9c4c2c0b0ffc10d4eaca31196d6b4e500e83b1fa4f2b7484da52fae803be7933d675010599303a9c79463a69c911ba3b3bc53"
	goldenMetaRootHex      = "a6006f1d0115af689095f203cbca9330"
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
	otherMeta, err := amp.BuildBlobMeta(bytes.NewReader(otherBlob), int64(len(otherBlob)), 0, amp.BlobChunkSizeLog2Min)
	if err != nil {
		t.Fatal(err)
	}
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
		{150 << 20, 20},     // 150 MB show scale → 1 MiB, 150 chunks
		{2 << 30, 20},       // 2 GiB → exactly 2048 chunks at 1 MiB
		{(2 << 30) + 1, 21}, // one byte over → 2 MiB
		{12 << 30, 23},      // 12 GiB → 8 MiB, 1536 chunks
		{32 << 30, 24},      // 32 GiB → 16 MiB, 2048 chunks
		{2 << 40, 30},       // 2 TiB → 1 GiB, 2048 chunks
		{100 << 40, 30},     // beyond target at max exponent — capped
		{3, 20},             // tiny → floor (caller sees single-chunk)
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
