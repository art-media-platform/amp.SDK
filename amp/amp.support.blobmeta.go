package amp

import (
	"bytes"
	"encoding/binary"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"google.golang.org/protobuf/encoding/protowire"
)

// ── BlobMeta: the transfer/verification manifest ────────────────────────
//
// Every blob spanning more than one grain carries a BlobMeta manifest over
// its STORED bytes (ciphertext for a sealed blob), minted in the store's
// single write pass and kept as a companion object beside the blob.  The
// ref commits to it via MetaRoot — the leading 16 bytes of the ref's
// HashKit digest over the meta's canonical encoding — and the ref rides a
// member-signed TxMsg, so a receiver verifies the meta against a signed
// commitment before trusting any chunk (SD-planet-storage §13.10).
//
// The chunk hash is two-level (composable):
//
//	fine digest = H(one grain of stored bytes)          level 0, untagged
//	meta entry  = H(composeTag ‖ the chunk's fine-digest run)   level 1
//
// The rule is uniform at every exponent — a chunk of one grain is still
// the tagged hash of its one-digest run.  The tag makes the two levels'
// input spaces disjoint, so a digest run can never collide with raw
// stored bytes.  Grain digests are derived on demand by any full holder
// (never stored): a low-bandwidth receiver pulls a chunk's run, verifies
// it against the meta entry, then verifies each arriving grain — the
// grain, not the chunk, is the narrow-link retry/resume quantum.  Entry
// width follows the HashKit (BlobMetaHashSize today; a wider kit is a
// registration, not a format change).

const (
	// BlobMetaHashSize is one BlobMeta entry: the leading bytes of the
	// tagged HashKit digest over the chunk's grain-digest run.
	BlobMetaHashSize = 32

	// BlobGrainSizeLog2 fixes the fine-grain quantum at 4 KiB: the level-0
	// hashing unit, the narrow-link (RF-class) verify/retry/resume quantum,
	// and the meta floor — a blob at or under one grain carries no meta.
	// 4 KiB keeps a grain retry at seconds even on baud-class links, gives
	// message-scale (4 KB+) payloads grain resume, and is page/FS-block
	// aligned.  Grain digests are transient — derived, sent, verified,
	// discarded — so grain size never costs stored or meta bytes (§13.10).
	BlobGrainSizeLog2 = 12

	// BlobChunkSizeLog2Min floors the encoder-chosen meta chunk size at 1 MiB.
	// What the floor buys (the authoritative rationale — other sites reference
	// it, never restate): it bounds the meta's weight at 32 B per MiB of
	// stored bytes (~0.003%) and keeps the chunk — the broadband transfer's
	// verify quantum — no finer than the 1 MiB default wire frame
	// (SD-planet-storage §13.10).  Meta chunk boundaries are stored-byte
	// shifts and do NOT align to seal AEAD frames (frame pitch carries
	// per-frame overhead); all verification is frame-blind.
	BlobChunkSizeLog2Min = 20

	// BlobChunkSizeLog2Max caps the meta chunk size at 1 GiB (TB-class assets).
	BlobChunkSizeLog2Max = 30

	// BlobMetaTargetChunks is the encoder's default sizing target: the
	// smallest chunk size in bounds that keeps the meta at or under this
	// many entries — ≤1 MiB of meta (one default wire frame), which holds
	// the 1 MiB chunk through 32 GiB blobs.
	BlobMetaTargetChunks = 32768

	// BlobLenMax bounds a blob's declared stored length (1 PiB): a generous,
	// finite guard so a garbled or hostile length can never drive an
	// unbounded transfer or overflow span arithmetic (§13.10).
	BlobLenMax = int64(1) << 50

	// blobMetaComposeTag prefixes every level-1 (digest-run) hash — the
	// domain separator that keeps run input disjoint from raw stored bytes.
	blobMetaComposeTag byte = 0x01

	// blobMetaSpillLen is the stored length past which BlobMetaBuilder
	// stops buffering grain digests (0.78% of stream) and switches to
	// incremental per-exponent lanes — bounding builder memory at ~16 MB
	// while staying single-pass at any size.
	blobMetaSpillLen = int64(2) << 30
)

// MetaRootUID is the ref's BlobMeta commitment; zero ⇒ a blob at or under
// one grain (no meta object — the whole transfer is one implicit chunk
// verified by BlobTag.UID).
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

// ── Meta-chunk address arithmetic ───────────────────────────────────────
//
// The single authoritative site for the meta's index ⇔ STORED-byte-offset
// mapping — a shift by the ref's ChunkSizeLog2 (SD-planet-storage §13.10).
// Serve-span resolution, the wire frame address, and staging absorb all
// address this space.  Meta chunks are NOT aligned to seal AEAD frames
// (frame pitch carries per-frame overhead); no caller may assume otherwise.

// BlobChunkOffset is the stored-byte offset of a meta chunk index (§13.10).
func BlobChunkOffset(chunkIndex uint64, chunkSizeLog2 uint32) int64 {
	return int64(chunkIndex) << chunkSizeLog2
}

// BlobChunkIndex is the meta chunk index containing a stored-byte position
// (§13.10).
func BlobChunkIndex(position int64, chunkSizeLog2 uint32) uint64 {
	return uint64(position) >> chunkSizeLog2
}

// BlobChunkRemaining is the byte count from a stored-byte position to the
// end of its meta chunk — a full chunk when the position sits on a boundary
// (§13.10).
func BlobChunkRemaining(position int64, chunkSizeLog2 uint32) int64 {
	chunkSize := int64(1) << chunkSizeLog2
	return chunkSize - position&(chunkSize-1)
}

// BlobChunkAligned reports whether a stored-byte position sits on a meta
// chunk boundary (§13.10).
func BlobChunkAligned(position int64, chunkSizeLog2 uint32) bool {
	return position&((int64(1)<<chunkSizeLog2)-1) == 0
}

// BlobWireAddress is the wire frame's (chunkIndex, offsetInChunk) address of
// a stored-byte position (§13.10; frame layout in SD-security-sync §13.8).
// ChunkSizeLog2 0 = a blob with no meta: one implicit chunk, the position is
// the in-chunk offset.
func BlobWireAddress(position int64, chunkSizeLog2 uint32) (chunkIndex, offsetInChunk uint64) {
	if chunkSizeLog2 == 0 {
		return 0, uint64(position)
	}
	return uint64(position) >> chunkSizeLog2, uint64(position) & ((1 << chunkSizeLog2) - 1)
}

// BlobPullSpan resolves a {ChunkBegin, ChunkCount} pull against a blob of
// blobLen stored bytes: the span's start offset and byte length on the
// meta's index space (§13.10; ChunkCount 0 = through end-of-blob).
// ok=false ⇒ a malformed exponent or a span outside the blob (chunk indexes
// are bounded by BlobLenMax so the shift can never overflow).
func BlobPullSpan(blobLen int64, chunkSizeLog2 uint32, chunkBegin, chunkCount uint64) (startOffset, spanLen int64, ok bool) {
	if chunkSizeLog2 >= 63 ||
		(chunkSizeLog2 > 0 && chunkBegin > uint64(BlobLenMax)>>chunkSizeLog2) ||
		(chunkSizeLog2 == 0 && chunkBegin != 0) {
		return 0, 0, false
	}
	startOffset = BlobChunkOffset(chunkBegin, chunkSizeLog2)
	if startOffset >= blobLen && blobLen > 0 {
		return 0, 0, false
	}
	spanLen = blobLen - startOffset
	if chunkCount > 0 {
		if requested := int64(chunkCount) << chunkSizeLog2; requested < spanLen {
			spanLen = requested
		}
	}
	return startOffset, spanLen, true
}

// ── Two-level chunk hashing (§13.10) ────────────────────────────────────

// newBlobHashKit resolves a HashKit for meta hashing, enforcing the entry
// width floor.
func newBlobHashKit(kitID safe.HashKitID) (safe.HashKit, error) {
	kit, err := safe.NewHashKit(kitID)
	if err != nil {
		return kit, err
	}
	if kit.HashSz < BlobMetaHashSize {
		return kit, status.Code_BadRequest.Errorf("amp: HashKit %v digest %d < meta entry size %d", kitID, kit.HashSz, BlobMetaHashSize)
	}
	return kit, nil
}

// BlobChunkHasher computes meta entries from a chunk's stored bytes — the
// one site holding the grain-then-compose mechanics every verifier shares.
// The caller streams one chunk's bytes through Write (any segmentation) and
// takes its entry with SumChunk; grain boundaries are handled internally.
type BlobChunkHasher struct {
	fine    safe.HashKit // level 0: raw grain bytes
	compose safe.HashKit // level 1: tagged fine-digest runs
	inGrain int64        // bytes fed into the current (partial) grain
	tagged  bool         // compose tag written for the current chunk
}

// NewBlobChunkHasher returns a chunk hasher under the given kit (zero =
// the registry default).
func NewBlobChunkHasher(kitID safe.HashKitID) (*BlobChunkHasher, error) {
	fine, err := newBlobHashKit(kitID)
	if err != nil {
		return nil, err
	}
	compose, err := newBlobHashKit(kitID)
	if err != nil {
		return nil, err
	}
	hasher := &BlobChunkHasher{
		fine:    fine,
		compose: compose,
	}
	return hasher, nil
}

// Write streams stored bytes of the current chunk (io.Writer; never errors).
func (bch *BlobChunkHasher) Write(chunk []byte) (int, error) {
	written := len(chunk)
	grainSize := int64(1) << BlobGrainSizeLog2
	for len(chunk) > 0 {
		span := min(grainSize-bch.inGrain, int64(len(chunk)))
		bch.fine.Hasher.Write(chunk[:span])
		bch.inGrain += span
		chunk = chunk[span:]
		if bch.inGrain == grainSize {
			bch.finishGrain()
		}
	}
	return written, nil
}

// finishGrain closes the current grain and feeds its digest to the run.
func (bch *BlobChunkHasher) finishGrain() {
	digest := bch.fine.Hasher.Sum(nil)
	bch.fine.Hasher.Reset()
	bch.inGrain = 0
	if !bch.tagged {
		bch.compose.Hasher.Reset()
		bch.compose.Hasher.Write([]byte{blobMetaComposeTag})
		bch.tagged = true
	}
	bch.compose.Hasher.Write(digest[:BlobMetaHashSize])
}

// SumChunk closes the current chunk — flushing a partial final grain — and
// returns its meta entry, resetting for the next chunk.
func (bch *BlobChunkHasher) SumChunk() []byte {
	if bch.inGrain > 0 || !bch.tagged {
		bch.finishGrain()
	}
	entry := bch.compose.Hasher.Sum(nil)[:BlobMetaHashSize]
	bch.tagged = false
	return entry
}

// Reset discards any partial chunk state.
func (bch *BlobChunkHasher) Reset() {
	bch.fine.Hasher.Reset()
	bch.inGrain = 0
	bch.tagged = false
}

// BlobGrainRun derives a chunk's fine-digest run from its stored bytes —
// what a full holder sends ahead of grains to a narrow-link receiver.  The
// run is transient: derived, sent, verified, discarded (§13.10).
func BlobGrainRun(kitID safe.HashKitID, chunk []byte) ([]byte, error) {
	fine, err := newBlobHashKit(kitID)
	if err != nil {
		return nil, err
	}
	grainSize := int64(1) << BlobGrainSizeLog2
	grainCount := (int64(len(chunk)) + grainSize - 1) >> BlobGrainSizeLog2
	run := make([]byte, 0, grainCount*BlobMetaHashSize)
	for len(chunk) > 0 {
		span := min(grainSize, int64(len(chunk)))
		fine.Hasher.Reset()
		fine.Hasher.Write(chunk[:span])
		run = append(run, fine.Hasher.Sum(nil)[:BlobMetaHashSize]...)
		chunk = chunk[span:]
	}
	return run, nil
}

// VerifyGrainRun checks a received fine-digest run against its chunk's meta
// entry — the narrow-link receiver's gate before trusting any grain.  A
// verified run then verifies each arriving grain by fine digest alone.
func VerifyGrainRun(kitID safe.HashKitID, entry []byte, run []byte) error {
	if len(run) == 0 || len(run)%BlobMetaHashSize != 0 {
		return status.Code_AuthFailed.Errorf("amp: VerifyGrainRun: run length %d is not a digest multiple", len(run))
	}
	compose, err := newBlobHashKit(kitID)
	if err != nil {
		return err
	}
	compose.Hasher.Write([]byte{blobMetaComposeTag})
	compose.Hasher.Write(run)
	if !bytes.Equal(compose.Hasher.Sum(nil)[:BlobMetaHashSize], entry) {
		return status.Code_AuthFailed.Error("amp: VerifyGrainRun: run does not match the meta entry")
	}
	return nil
}

// ── Streaming meta mint ─────────────────────────────────────────────────

// blobMetaLane accumulates one candidate exponent's entries once the
// builder spills out of digest buffering (storedLen > blobMetaSpillLen).
type blobMetaLane struct {
	compose safe.HashKit // level-1 hasher for the run in progress
	entries []byte       // emitted meta entries
	grains  uint64       // fine digests fed into the run in progress
	tagged  bool         // compose tag written for the run in progress
	dead    bool         // exponent ruled out by the grown storedLen
}

// BlobMetaBuilder mints a BlobMeta in the store's single write pass — the
// one authoritative mint site for every publish path.  Stream the STORED
// bytes through Write (any segmentation), then Finish; a nil meta means the
// blob is at or under one grain and carries none.  One-shot; not reusable.
//
// Internals: grain digests are buffered (32 B per grain, ≤16 MB) and the
// winning exponent's entries composed at Finish; past blobMetaSpillLen the
// buffer replays into per-exponent lanes — dead exponents pruned as the
// length grows — so memory stays bounded and the pass count stays one at
// any size.
type BlobMetaBuilder struct {
	kitID   safe.HashKitID
	fine    safe.HashKit   // level 0: raw grain bytes
	inGrain int64          // bytes fed into the current (partial) grain
	total   int64          // stored bytes seen
	runBuf  []byte         // buffered grain digests (until spill)
	lanes   []blobMetaLane // per-exponent lanes (nil until spill)
}

// NewBlobMetaBuilder returns a builder under the given kit (zero = the
// registry default).
func NewBlobMetaBuilder(kitID safe.HashKitID) (*BlobMetaBuilder, error) {
	fine, err := newBlobHashKit(kitID)
	if err != nil {
		return nil, err
	}
	builder := &BlobMetaBuilder{
		kitID: kitID,
		fine:  fine,
	}
	return builder, nil
}

// Write streams stored bytes (io.Writer).
func (bld *BlobMetaBuilder) Write(stored []byte) (int, error) {
	if bld.total+int64(len(stored)) > BlobLenMax {
		return 0, status.Code_BadRequest.Errorf("amp: BlobMetaBuilder: stored length exceeds BlobLenMax (%d)", BlobLenMax)
	}
	written := len(stored)
	grainSize := int64(1) << BlobGrainSizeLog2
	for len(stored) > 0 {
		span := min(grainSize-bld.inGrain, int64(len(stored)))
		bld.fine.Hasher.Write(stored[:span])
		bld.inGrain += span
		bld.total += span
		stored = stored[span:]
		if bld.inGrain == grainSize {
			bld.absorbGrain()
		}
	}
	return written, nil
}

// absorbGrain closes the current grain and routes its digest.
func (bld *BlobMetaBuilder) absorbGrain() {
	digest := bld.fine.Hasher.Sum(nil)
	bld.fine.Hasher.Reset()
	bld.inGrain = 0

	if bld.lanes == nil {
		bld.runBuf = append(bld.runBuf, digest[:BlobMetaHashSize]...)
		if bld.total > blobMetaSpillLen {
			bld.spill()
		}
		return
	}
	bld.feedLanes(digest[:BlobMetaHashSize])
	bld.pruneLanes()
}

// spill replays the buffered digests into per-exponent lanes and frees the
// buffer — the giant-blob posture.
func (bld *BlobMetaBuilder) spill() {
	laneCount := BlobChunkSizeLog2Max - BlobChunkSizeLog2Min + 1
	bld.lanes = make([]blobMetaLane, laneCount)
	for ii := range bld.lanes {
		kit, err := newBlobHashKit(bld.kitID)
		if err != nil {
			// The kit already resolved at construction; a registry mutation
			// mid-stream is not a supported state.
			panic(err)
		}
		bld.lanes[ii].compose = kit
	}
	buffered := bld.runBuf
	bld.runBuf = nil
	for begin := 0; begin < len(buffered); begin += BlobMetaHashSize {
		bld.feedLanes(buffered[begin : begin+BlobMetaHashSize])
	}
	bld.pruneLanes()
}

// feedLanes routes one grain digest into every live lane.
func (bld *BlobMetaBuilder) feedLanes(digest []byte) {
	for ii := range bld.lanes {
		lane := &bld.lanes[ii]
		if lane.dead {
			continue
		}
		if !lane.tagged {
			lane.compose.Hasher.Reset()
			lane.compose.Hasher.Write([]byte{blobMetaComposeTag})
			lane.tagged = true
		}
		lane.compose.Hasher.Write(digest)
		lane.grains++
		exp := uint32(BlobChunkSizeLog2Min + ii)
		if lane.grains == blobGrainsPerChunk(exp) {
			lane.entries = append(lane.entries, lane.compose.Hasher.Sum(nil)[:BlobMetaHashSize]...)
			lane.tagged = false
			lane.grains = 0
		}
	}
}

// pruneLanes kills exponents the grown length has ruled out; the max
// exponent never dies.
func (bld *BlobMetaBuilder) pruneLanes() {
	for ii := range bld.lanes[:len(bld.lanes)-1] {
		lane := &bld.lanes[ii]
		if lane.dead {
			continue
		}
		if blobLaneDead(bld.total, uint32(BlobChunkSizeLog2Min+ii)) {
			lane.dead = true
			lane.entries = nil
		}
	}
}

// blobLaneDead reports whether an exponent can no longer be the chosen one
// for a stream already storedLen bytes long — its entry count exceeds the
// target, and ChooseBlobChunkSizeLog2 picks the smallest exponent that
// fits, so the lane can never win.
func blobLaneDead(storedLen int64, chunkSizeLog2 uint32) bool {
	return storedLen > int64(BlobMetaTargetChunks)<<chunkSizeLog2
}

// blobGrainsPerChunk is the fine-digest run length of a full chunk.
func blobGrainsPerChunk(chunkSizeLog2 uint32) uint64 {
	return uint64(1) << (chunkSizeLog2 - BlobGrainSizeLog2)
}

// Finish closes the stream and mints the meta — nil when the blob is at or
// under one grain (no meta object; the whole transfer is verified by
// BlobTag.UID alone).
func (bld *BlobMetaBuilder) Finish() (*BlobMeta, error) {
	if bld.inGrain > 0 {
		bld.absorbGrain()
	}
	if bld.total <= int64(1)<<BlobGrainSizeLog2 {
		return nil, nil
	}
	exp := ChooseBlobChunkSizeLog2(bld.total)
	meta := &BlobMeta{
		ChunkSizeLog2: exp,
		TotalLen:      uint64(bld.total),
	}

	if bld.lanes == nil {
		// Compose the winning exponent's entries from the buffered digests.
		compose, err := newBlobHashKit(bld.kitID)
		if err != nil {
			return nil, err
		}
		runLen := int(blobGrainsPerChunk(exp)) * BlobMetaHashSize
		for begin := 0; begin < len(bld.runBuf); begin += runLen {
			end := min(begin+runLen, len(bld.runBuf))
			compose.Hasher.Reset()
			compose.Hasher.Write([]byte{blobMetaComposeTag})
			compose.Hasher.Write(bld.runBuf[begin:end])
			meta.ChunkHashes = append(meta.ChunkHashes, compose.Hasher.Sum(nil)[:BlobMetaHashSize]...)
		}
		return meta, nil
	}

	lane := &bld.lanes[exp-BlobChunkSizeLog2Min]
	if lane.dead {
		return nil, status.Code_DataFailure.Errorf("amp: BlobMetaBuilder: chosen exponent %d was pruned — builder invariant broken", exp)
	}
	if lane.grains > 0 {
		lane.entries = append(lane.entries, lane.compose.Hasher.Sum(nil)[:BlobMetaHashSize]...)
		lane.tagged = false
		lane.grains = 0
	}
	meta.ChunkHashes = lane.entries
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

// ChunkUID is one chunk's hashname — the leading 16 bytes of its meta entry
// as a tag.UID, the swarm-addressable identity of that chunk (§13.10).
func (meta *BlobMeta) ChunkUID(chunkIndex uint64) tag.UID {
	entry := meta.ChunkHash(chunkIndex)
	return tag.UID{
		binary.BigEndian.Uint64(entry[0:8]),
		binary.BigEndian.Uint64(entry[8:16]),
	}
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
	if meta.TotalLen <= uint64(1)<<BlobGrainSizeLog2 {
		return status.Code_AuthFailed.Error("amp: VerifyBlobMeta: a blob at or under one grain carries no meta")
	}
	numChunks := BlobChunkCount(int64(meta.TotalLen), meta.ChunkSizeLog2)
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
	hasher, err := NewBlobChunkHasher(kitID)
	if err != nil {
		return err
	}
	hasher.Write(chunk)
	if !bytes.Equal(hasher.SumChunk(), meta.ChunkHash(chunkIndex)) {
		return status.Code_AuthFailed.Errorf("amp: VerifyChunk: chunk %d hash does not match its meta entry", chunkIndex)
	}
	return nil
}
