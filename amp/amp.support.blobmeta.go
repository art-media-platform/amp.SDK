package amp

import (
	"bytes"
	"encoding/binary"
	"io"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"google.golang.org/protobuf/encoding/protowire"
)

// ── BlobMeta: the transfer/verification manifest ────────────────────────
//
// Every multi-chunk blob carries a BlobMeta manifest over its STORED bytes
// (ciphertext for a sealed blob), computed at seal/store time and kept as a
// companion object beside the blob.  The ref commits to it via MetaRoot —
// the leading 16 bytes of the ref's HashKit digest over the meta's canonical
// encoding — and the ref rides a member-signed TxMsg, so a receiver verifies
// the meta against a signed commitment before trusting any chunk, then each
// arriving chunk against its meta hash (SD-planet-storage §13.10).

const (
	// BlobMetaHashSize is one BlobMeta entry: the leading bytes of the
	// ref's HashKit digest over one stored-byte chunk.
	BlobMetaHashSize = 32

	// BlobChunkSizeLog2Min floors the encoder-chosen meta chunk size at 1 MiB.
	// What the floor buys (the authoritative rationale — other sites reference
	// it, never restate): it bounds the meta's weight at 32 B per MiB of
	// stored bytes (~0.003%), makes every blob at or under 1 MiB single-chunk
	// (no meta object at all — the cheap small-asset case), and keeps the
	// chunk — the transfer's verify/retry quantum — no finer than the 1 MiB
	// default wire frame (SD-planet-storage §13.10).  Meta chunk boundaries
	// are stored-byte shifts and do NOT align to seal AEAD frames (frame
	// pitch carries per-frame overhead); all verification is frame-blind.
	BlobChunkSizeLog2Min = 20

	// BlobChunkSizeLog2Max caps the meta chunk size at 1 GiB (TB-class assets).
	BlobChunkSizeLog2Max = 30

	// BlobMetaTargetChunks is the encoder's default sizing target: the
	// smallest chunk size in bounds that keeps the meta at or under this many
	// entries (~1–2k chunks per blob).
	BlobMetaTargetChunks = 2048
)

// MetaRootUID is the ref's BlobMeta commitment; zero ⇒ single-chunk blob
// (no meta object — the whole transfer is one implicit chunk verified by
// BlobTag.UID).
func (ref *BlobRef) MetaRootUID() tag.UID {
	return tag.UID{ref.MetaRoot_0, ref.MetaRoot_1}
}

// HasBlobMeta reports whether this blob carries a BlobMeta commitment.
func (ref *BlobRef) HasBlobMeta() bool {
	return ref.MetaRoot_0 != 0 || ref.MetaRoot_1 != 0
}

// ChooseBlobChunkSizeLog2 returns the encoder-default power-of-2 exponent for
// a blob of storedLen bytes: the smallest exponent within bounds that keeps
// the meta at or under BlobMetaTargetChunks entries.  Media type may bias
// the choice; the encoder owns it — this is the length-based default.
func ChooseBlobChunkSizeLog2(storedLen int64) uint32 {
	exp := uint32(BlobChunkSizeLog2Min)
	for exp < BlobChunkSizeLog2Max && (storedLen+(1<<exp)-1)>>exp > BlobMetaTargetChunks {
		exp++
	}
	return exp
}

// BlobChunkCount returns how many meta chunks a blob of storedLen bytes spans
// at the given exponent.  A count of one (or zero) means single-chunk: no
// meta object, zero MetaRoot / ChunkSizeLog2 on the ref.
func BlobChunkCount(storedLen int64, chunkSizeLog2 uint32) uint64 {
	if storedLen <= 0 {
		return 0
	}
	return uint64(storedLen+(1<<chunkSizeLog2)-1) >> chunkSizeLog2
}

// BuildBlobMeta reads exactly storedLen bytes of STORED content from src
// and returns its BlobMeta.  The caller chooses chunkSizeLog2 (see
// ChooseBlobChunkSizeLog2) and only calls this for multi-chunk blobs — a
// single-chunk blob carries no meta.
func BuildBlobMeta(src io.Reader, storedLen int64, kitID safe.HashKitID, chunkSizeLog2 uint32) (*BlobMeta, error) {
	if chunkSizeLog2 < BlobChunkSizeLog2Min || chunkSizeLog2 > BlobChunkSizeLog2Max {
		return nil, status.Code_BadRequest.Errorf("amp: BuildBlobMeta: ChunkSizeLog2 %d out of bounds [%d,%d]", chunkSizeLog2, BlobChunkSizeLog2Min, BlobChunkSizeLog2Max)
	}
	numChunks := BlobChunkCount(storedLen, chunkSizeLog2)
	if numChunks < 2 {
		return nil, status.Code_BadRequest.Errorf("amp: BuildBlobMeta: %d bytes is single-chunk at 2^%d — no meta", storedLen, chunkSizeLog2)
	}
	kit, err := safe.NewHashKit(kitID)
	if err != nil {
		return nil, err
	}
	if kit.HashSz < BlobMetaHashSize {
		return nil, status.Code_BadRequest.Errorf("amp: BuildBlobMeta: HashKit %v digest %d < meta entry size %d", kitID, kit.HashSz, BlobMetaHashSize)
	}
	meta := &BlobMeta{
		ChunkSizeLog2: chunkSizeLog2,
		TotalLen:      uint64(storedLen),
		ChunkHashes:   make([]byte, 0, numChunks*BlobMetaHashSize),
	}
	chunkSize := int64(1) << chunkSizeLog2
	remain := storedLen
	for remain > 0 {
		span := min(chunkSize, remain)
		kit.Hasher.Reset()
		if _, err := io.CopyN(kit.Hasher, src, span); err != nil {
			return nil, status.Code_DataFailure.Errorf("amp: BuildBlobMeta: short read: %v", err)
		}
		meta.ChunkHashes = append(meta.ChunkHashes, kit.Hasher.Sum(nil)[:BlobMetaHashSize]...)
		remain -= span
	}
	return meta, nil
}

// CanonicalBytes is the encoding MetaRoot commits to: this proto's fields in
// order, standard proto3 wire form (zero-valued fields omitted) — pinned
// against cross-language marshal drift by golden fixture.
func (meta *BlobMeta) CanonicalBytes() []byte {
	canon := make([]byte, 0, 16+len(meta.ChunkHashes))
	if meta.ChunkSizeLog2 != 0 {
		canon = protowire.AppendTag(canon, 1, protowire.VarintType)
		canon = protowire.AppendVarint(canon, uint64(meta.ChunkSizeLog2))
	}
	if meta.TotalLen != 0 {
		canon = protowire.AppendTag(canon, 2, protowire.VarintType)
		canon = protowire.AppendVarint(canon, meta.TotalLen)
	}
	if len(meta.ChunkHashes) > 0 {
		canon = protowire.AppendTag(canon, 3, protowire.BytesType)
		canon = protowire.AppendBytes(canon, meta.ChunkHashes)
	}
	return canon
}

// BlobMetaWireSize returns the exact canonical (wire) byte length of the
// BlobMeta for a blob of storedLen stored bytes at the given exponent — computable by a
// receiver from the signed ref alone, which is what bounds a meta pull before any
// byte arrives.
func BlobMetaWireSize(storedLen int64, chunkSizeLog2 uint32) int64 {
	hashesLen := BlobChunkCount(storedLen, chunkSizeLog2) * BlobMetaHashSize
	size := 0
	if chunkSizeLog2 != 0 {
		size += 1 + protowire.SizeVarint(uint64(chunkSizeLog2))
	}
	if storedLen != 0 {
		size += 1 + protowire.SizeVarint(uint64(storedLen))
	}
	if hashesLen > 0 {
		size += 1 + protowire.SizeVarint(hashesLen) + int(hashesLen)
	}
	return int64(size)
}

// RootUID returns the meta's commitment under the given HashKit: the leading
// 16 bytes of the digest over CanonicalBytes — what BlobRef.MetaRoot carries.
func (meta *BlobMeta) RootUID(kitID safe.HashKitID) (tag.UID, error) {
	digest, err := hashBytes(kitID, meta.CanonicalBytes())
	if err != nil {
		return tag.UID{}, err
	}
	return tag.UID{
		binary.BigEndian.Uint64(digest[0:8]),
		binary.BigEndian.Uint64(digest[8:16]),
	}, nil
}

// NumChunks is the meta's entry count.
func (meta *BlobMeta) NumChunks() uint64 {
	return uint64(len(meta.ChunkHashes) / BlobMetaHashSize)
}

// ChunkHash returns the meta entry for one chunk index.
func (meta *BlobMeta) ChunkHash(chunkIndex uint64) []byte {
	begin := chunkIndex * BlobMetaHashSize
	return meta.ChunkHashes[begin : begin+BlobMetaHashSize]
}

// ChunkSpan returns the stored-byte offset and length of one chunk —
// index ⇔ offset is a shift by ChunkSizeLog2; the final chunk is the remainder.
func (meta *BlobMeta) ChunkSpan(chunkIndex uint64) (offset int64, length int64) {
	offset = int64(chunkIndex) << meta.ChunkSizeLog2
	length = int64(1) << meta.ChunkSizeLog2
	if remainder := int64(meta.TotalLen) - offset; remainder < length {
		length = remainder
	}
	return offset, length
}

// SetBlobMeta stamps the ref's BlobMeta commitment from a built meta.
func (ref *BlobRef) SetBlobMeta(meta *BlobMeta, kitID safe.HashKitID) error {
	root, err := meta.RootUID(kitID)
	if err != nil {
		return err
	}
	ref.MetaRoot_0 = root[0]
	ref.MetaRoot_1 = root[1]
	ref.ChunkSizeLog2 = meta.ChunkSizeLog2
	return nil
}

// VerifyBlobMeta checks a fetched meta against the ref's signed commitment —
// the receiver-side gate before any chunk is trusted.  Verifies shape (exponent
// bounds and match, TotalLen against BlobTag.I, entry count against the span
// arithmetic) and the root commitment.
func (ref *BlobRef) VerifyBlobMeta(meta *BlobMeta) error {
	if !ref.HasBlobMeta() {
		return status.Code_BadRequest.Error("amp: VerifyBlobMeta: ref carries no BlobMeta commitment")
	}
	if meta == nil {
		return status.Code_BadRequest.Error("amp: VerifyBlobMeta: nil meta")
	}
	if meta.ChunkSizeLog2 != ref.ChunkSizeLog2 {
		return status.Code_AuthFailed.Errorf("amp: VerifyBlobMeta: ChunkSizeLog2 %d != ref's %d", meta.ChunkSizeLog2, ref.ChunkSizeLog2)
	}
	if meta.ChunkSizeLog2 < BlobChunkSizeLog2Min || meta.ChunkSizeLog2 > BlobChunkSizeLog2Max {
		return status.Code_AuthFailed.Errorf("amp: VerifyBlobMeta: ChunkSizeLog2 %d out of bounds [%d,%d]", meta.ChunkSizeLog2, BlobChunkSizeLog2Min, BlobChunkSizeLog2Max)
	}
	if ref.BlobTag != nil && meta.TotalLen != uint64(ref.BlobTag.I) {
		return status.Code_AuthFailed.Errorf("amp: VerifyBlobMeta: TotalLen %d != stored length %d", meta.TotalLen, ref.BlobTag.I)
	}
	numChunks := BlobChunkCount(int64(meta.TotalLen), meta.ChunkSizeLog2)
	if numChunks < 2 {
		return status.Code_AuthFailed.Error("amp: VerifyBlobMeta: single-chunk blob carries no meta")
	}
	if len(meta.ChunkHashes) != int(numChunks)*BlobMetaHashSize {
		return status.Code_AuthFailed.Errorf("amp: VerifyBlobMeta: %d hash bytes != %d chunks × %d", len(meta.ChunkHashes), numChunks, BlobMetaHashSize)
	}
	root, err := meta.RootUID(ref.HashKitID)
	if err != nil {
		return err
	}
	if root != ref.MetaRootUID() {
		return status.Code_AuthFailed.Error("amp: VerifyBlobMeta: meta root does not match the ref's commitment")
	}
	return nil
}

// VerifyChunk checks one arriving chunk's stored bytes against its meta entry —
// the receiver's per-chunk gate (verify-then-share: only verified chunks are
// persisted or served onward).
func (meta *BlobMeta) VerifyChunk(chunkIndex uint64, chunk []byte, kitID safe.HashKitID) error {
	if chunkIndex >= meta.NumChunks() {
		return status.Code_AuthFailed.Errorf("amp: VerifyChunk: index %d out of range (%d chunks)", chunkIndex, meta.NumChunks())
	}
	_, wantLen := meta.ChunkSpan(chunkIndex)
	if int64(len(chunk)) != wantLen {
		return status.Code_AuthFailed.Errorf("amp: VerifyChunk: chunk %d is %d bytes, meta says %d", chunkIndex, len(chunk), wantLen)
	}
	digest, err := hashBytes(kitID, chunk)
	if err != nil {
		return err
	}
	if !bytes.Equal(digest[:BlobMetaHashSize], meta.ChunkHash(chunkIndex)) {
		return status.Code_AuthFailed.Errorf("amp: VerifyChunk: chunk %d hash does not match its meta entry", chunkIndex)
	}
	return nil
}
