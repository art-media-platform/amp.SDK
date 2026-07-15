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

// ── BlobChunkTable: the transfer/verification manifest ────────────────────────
//
// Every multi-chunk blob carries a chunk table over its STORED bytes
// (ciphertext for a sealed blob), computed at seal/store time and kept as a
// companion object beside the blob.  The ref commits to it via TableRoot —
// the leading 16 bytes of the ref's HashKit digest over the table's canonical
// encoding — and the ref rides a member-signed TxMsg, so a receiver verifies
// the table against a signed commitment before trusting any chunk, then each
// arriving chunk against its table hash (SD-planet-storage §13.10).

const (
	// BlobTableHashSize is one BlobChunkTable entry: the leading bytes of the
	// ref's HashKit digest over one stored-byte chunk.
	BlobTableHashSize = 32

	// BlobChunkSizeLog2Min floors the encoder-chosen table chunk size at 1 MiB —
	// the seal AEAD chunk — so a table chunk always covers whole AEAD frames.
	BlobChunkSizeLog2Min = 20

	// BlobChunkSizeLog2Max caps the table chunk size at 1 GiB (TB-class assets).
	BlobChunkSizeLog2Max = 30

	// BlobTableTargetChunks is the encoder's default sizing target: the
	// smallest chunk size in bounds that keeps the table at or under this many
	// entries (~1–2k chunks per blob).
	BlobTableTargetChunks = 2048
)

// TableRootUID is the ref's chunk-table commitment; zero ⇒ single-chunk blob
// (no table object — the whole transfer is one implicit chunk verified by
// BlobTag.UID).
func (ref *BlobRef) TableRootUID() tag.UID {
	return tag.UID{ref.TableRoot_0, ref.TableRoot_1}
}

// HasChunkTable reports whether this blob carries a chunk-table commitment.
func (ref *BlobRef) HasChunkTable() bool {
	return ref.TableRoot_0 != 0 || ref.TableRoot_1 != 0
}

// ChooseBlobChunkSizeLog2 returns the encoder-default power-of-2 exponent for
// a blob of storedLen bytes: the smallest exponent within bounds that keeps
// the table at or under BlobTableTargetChunks entries.  Media type may bias
// the choice; the encoder owns it — this is the length-based default.
func ChooseBlobChunkSizeLog2(storedLen int64) uint32 {
	exp := uint32(BlobChunkSizeLog2Min)
	for exp < BlobChunkSizeLog2Max && (storedLen+(1<<exp)-1)>>exp > BlobTableTargetChunks {
		exp++
	}
	return exp
}

// BlobChunkCount returns how many table chunks a blob of storedLen bytes spans
// at the given exponent.  A count of one (or zero) means single-chunk: no
// table object, zero TableRoot / ChunkSizeLog2 on the ref.
func BlobChunkCount(storedLen int64, chunkSizeLog2 uint32) uint64 {
	if storedLen <= 0 {
		return 0
	}
	return uint64(storedLen+(1<<chunkSizeLog2)-1) >> chunkSizeLog2
}

// BuildBlobChunkTable reads exactly storedLen bytes of STORED content from src
// and returns its chunk table.  The caller chooses chunkSizeLog2 (see
// ChooseBlobChunkSizeLog2) and only calls this for multi-chunk blobs — a
// single-chunk blob carries no table.
func BuildBlobChunkTable(src io.Reader, storedLen int64, kitID safe.HashKitID, chunkSizeLog2 uint32) (*BlobChunkTable, error) {
	if chunkSizeLog2 < BlobChunkSizeLog2Min || chunkSizeLog2 > BlobChunkSizeLog2Max {
		return nil, status.Code_BadRequest.Errorf("amp: BuildBlobChunkTable: ChunkSizeLog2 %d out of bounds [%d,%d]", chunkSizeLog2, BlobChunkSizeLog2Min, BlobChunkSizeLog2Max)
	}
	numChunks := BlobChunkCount(storedLen, chunkSizeLog2)
	if numChunks < 2 {
		return nil, status.Code_BadRequest.Errorf("amp: BuildBlobChunkTable: %d bytes is single-chunk at 2^%d — no table", storedLen, chunkSizeLog2)
	}
	kit, err := safe.NewHashKit(kitID)
	if err != nil {
		return nil, err
	}
	if kit.HashSz < BlobTableHashSize {
		return nil, status.Code_BadRequest.Errorf("amp: BuildBlobChunkTable: HashKit %v digest %d < table entry size %d", kitID, kit.HashSz, BlobTableHashSize)
	}
	table := &BlobChunkTable{
		ChunkSizeLog2: chunkSizeLog2,
		TotalLen:      uint64(storedLen),
		ChunkHashes:   make([]byte, 0, numChunks*BlobTableHashSize),
	}
	chunkSize := int64(1) << chunkSizeLog2
	remain := storedLen
	for remain > 0 {
		span := min(chunkSize, remain)
		kit.Hasher.Reset()
		if _, err := io.CopyN(kit.Hasher, src, span); err != nil {
			return nil, status.Code_DataFailure.Errorf("amp: BuildBlobChunkTable: short read: %v", err)
		}
		table.ChunkHashes = append(table.ChunkHashes, kit.Hasher.Sum(nil)[:BlobTableHashSize]...)
		remain -= span
	}
	return table, nil
}

// CanonicalBytes is the encoding TableRoot commits to: this proto's fields in
// order, standard proto3 wire form (zero-valued fields omitted) — pinned
// against cross-language marshal drift by golden fixture.
func (table *BlobChunkTable) CanonicalBytes() []byte {
	canon := make([]byte, 0, 16+len(table.ChunkHashes))
	if table.ChunkSizeLog2 != 0 {
		canon = protowire.AppendTag(canon, 1, protowire.VarintType)
		canon = protowire.AppendVarint(canon, uint64(table.ChunkSizeLog2))
	}
	if table.TotalLen != 0 {
		canon = protowire.AppendTag(canon, 2, protowire.VarintType)
		canon = protowire.AppendVarint(canon, table.TotalLen)
	}
	if len(table.ChunkHashes) > 0 {
		canon = protowire.AppendTag(canon, 3, protowire.BytesType)
		canon = protowire.AppendBytes(canon, table.ChunkHashes)
	}
	return canon
}

// BlobChunkTableWireSize returns the exact canonical (wire) byte length of the chunk
// table for a blob of storedLen stored bytes at the given exponent — computable by a
// receiver from the signed ref alone, which is what bounds a table pull before any
// byte arrives.
func BlobChunkTableWireSize(storedLen int64, chunkSizeLog2 uint32) int64 {
	hashesLen := BlobChunkCount(storedLen, chunkSizeLog2) * BlobTableHashSize
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

// RootUID returns the table's commitment under the given HashKit: the leading
// 16 bytes of the digest over CanonicalBytes — what BlobRef.TableRoot carries.
func (table *BlobChunkTable) RootUID(kitID safe.HashKitID) (tag.UID, error) {
	digest, err := hashBytes(kitID, table.CanonicalBytes())
	if err != nil {
		return tag.UID{}, err
	}
	return tag.UID{
		binary.BigEndian.Uint64(digest[0:8]),
		binary.BigEndian.Uint64(digest[8:16]),
	}, nil
}

// NumChunks is the table's entry count.
func (table *BlobChunkTable) NumChunks() uint64 {
	return uint64(len(table.ChunkHashes) / BlobTableHashSize)
}

// ChunkHash returns the table entry for one chunk index.
func (table *BlobChunkTable) ChunkHash(chunkIndex uint64) []byte {
	begin := chunkIndex * BlobTableHashSize
	return table.ChunkHashes[begin : begin+BlobTableHashSize]
}

// ChunkSpan returns the stored-byte offset and length of one chunk —
// index ⇔ offset is a shift by ChunkSizeLog2; the final chunk is the remainder.
func (table *BlobChunkTable) ChunkSpan(chunkIndex uint64) (offset int64, length int64) {
	offset = int64(chunkIndex) << table.ChunkSizeLog2
	length = int64(1) << table.ChunkSizeLog2
	if remainder := int64(table.TotalLen) - offset; remainder < length {
		length = remainder
	}
	return offset, length
}

// SetChunkTable stamps the ref's chunk-table commitment from a built table.
func (ref *BlobRef) SetChunkTable(table *BlobChunkTable, kitID safe.HashKitID) error {
	root, err := table.RootUID(kitID)
	if err != nil {
		return err
	}
	ref.TableRoot_0 = root[0]
	ref.TableRoot_1 = root[1]
	ref.ChunkSizeLog2 = table.ChunkSizeLog2
	return nil
}

// VerifyChunkTable checks a fetched table against the ref's signed commitment —
// the receiver-side gate before any chunk is trusted.  Verifies shape (exponent
// bounds and match, TotalLen against BlobTag.I, entry count against the span
// arithmetic) and the root commitment.
func (ref *BlobRef) VerifyChunkTable(table *BlobChunkTable) error {
	if !ref.HasChunkTable() {
		return status.Code_BadRequest.Error("amp: VerifyChunkTable: ref carries no chunk-table commitment")
	}
	if table == nil {
		return status.Code_BadRequest.Error("amp: VerifyChunkTable: nil table")
	}
	if table.ChunkSizeLog2 != ref.ChunkSizeLog2 {
		return status.Code_AuthFailed.Errorf("amp: VerifyChunkTable: ChunkSizeLog2 %d != ref's %d", table.ChunkSizeLog2, ref.ChunkSizeLog2)
	}
	if table.ChunkSizeLog2 < BlobChunkSizeLog2Min || table.ChunkSizeLog2 > BlobChunkSizeLog2Max {
		return status.Code_AuthFailed.Errorf("amp: VerifyChunkTable: ChunkSizeLog2 %d out of bounds [%d,%d]", table.ChunkSizeLog2, BlobChunkSizeLog2Min, BlobChunkSizeLog2Max)
	}
	if ref.BlobTag != nil && table.TotalLen != uint64(ref.BlobTag.I) {
		return status.Code_AuthFailed.Errorf("amp: VerifyChunkTable: TotalLen %d != stored length %d", table.TotalLen, ref.BlobTag.I)
	}
	numChunks := BlobChunkCount(int64(table.TotalLen), table.ChunkSizeLog2)
	if numChunks < 2 {
		return status.Code_AuthFailed.Error("amp: VerifyChunkTable: single-chunk blob carries no table")
	}
	if len(table.ChunkHashes) != int(numChunks)*BlobTableHashSize {
		return status.Code_AuthFailed.Errorf("amp: VerifyChunkTable: %d hash bytes != %d chunks × %d", len(table.ChunkHashes), numChunks, BlobTableHashSize)
	}
	root, err := table.RootUID(ref.HashKitID)
	if err != nil {
		return err
	}
	if root != ref.TableRootUID() {
		return status.Code_AuthFailed.Error("amp: VerifyChunkTable: table root does not match the ref's commitment")
	}
	return nil
}

// VerifyChunk checks one arriving chunk's stored bytes against its table entry —
// the receiver's per-chunk gate (verify-then-share: only verified chunks are
// persisted or served onward).
func (table *BlobChunkTable) VerifyChunk(chunkIndex uint64, chunk []byte, kitID safe.HashKitID) error {
	if chunkIndex >= table.NumChunks() {
		return status.Code_AuthFailed.Errorf("amp: VerifyChunk: index %d out of range (%d chunks)", chunkIndex, table.NumChunks())
	}
	_, wantLen := table.ChunkSpan(chunkIndex)
	if int64(len(chunk)) != wantLen {
		return status.Code_AuthFailed.Errorf("amp: VerifyChunk: chunk %d is %d bytes, table says %d", chunkIndex, len(chunk), wantLen)
	}
	digest, err := hashBytes(kitID, chunk)
	if err != nil {
		return err
	}
	if !bytes.Equal(digest[:BlobTableHashSize], table.ChunkHash(chunkIndex)) {
		return status.Code_AuthFailed.Errorf("amp: VerifyChunk: chunk %d hash does not match its table entry", chunkIndex)
	}
	return nil
}
