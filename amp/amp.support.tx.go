package amp

import (
	"encoding/binary"
	"io"
	"sort"
	"unsafe"

	"github.com/art-media-platform/amp.SDK/stdlib/data"
	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"google.golang.org/protobuf/proto"
)

// TxDataStore is a message packet sent to / from a client.
// It leads with a fixed-size header (TxPreambleSize).
type TxDataStore []byte

// TxPreamble is the fixed-size header that leads every TxMsg.
type TxPreamble [TxPreambleSize]byte

func (preamble TxPreamble) TxHeadLen() int {
	return int(binary.BigEndian.Uint32(preamble[4:8]))
}

func (preamble TxPreamble) TxDataLen() int {
	return int(binary.BigEndian.Uint32(preamble[8:12]))
}

func (preamble TxPreamble) TxSignatureSize() int {
	return int(binary.BigEndian.Uint16(preamble[12:14]))
}

// TxNew is the single TxMsg constructor — internal buffer reuse, if ever
// warranted, lands here without touching callers.
func TxNew() *TxMsg {
	return &TxMsg{}
}

func (tx *TxEnvelope) TxID() tag.UID {
	return tag.UID{tx.TxID_0, tx.TxID_1}
}

func (tx *TxEnvelope) SetTxID(ID tag.UID) {
	tx.TxID_0 = ID[0]
	tx.TxID_1 = ID[1]
}

// MemberProofInput returns the bytes the MemberProof HMAC commits to: the
// TxID as 16 big-endian bytes.  The proof producer and every verify site must
// consume this one encoding.
func (tx *TxEnvelope) MemberProofInput() []byte {
	return tx.TxID().AppendTo(nil)
}

// PlanetID returns the planet this tx applies to.
func (tx *TxEnvelope) PlanetID() tag.UID {
	return tag.UID{tx.Planet_0, tx.Planet_1}
}

// SetPlanetID sets the planet this tx applies to — the routing lever.
func (tx *TxEnvelope) SetPlanetID(planetID tag.UID) {
	tx.Planet_0 = planetID[0]
	tx.Planet_1 = planetID[1]
}

// EpochID returns the epoch (planet or channel) keying this tx's payload;
// zero = planet-public.
func (tx *TxEnvelope) EpochID() tag.UID {
	return tag.UID{tx.Epoch_0, tx.Epoch_1}
}

// SetEpochID sets the epoch that keys this tx's payload — the privacy lever,
// independent of routing (Planet) and author (FromID).  Zero leaves the tx
// planet-public.
func (tx *TxEnvelope) SetEpochID(epochID tag.UID) {
	tx.Epoch_0 = epochID[0]
	tx.Epoch_1 = epochID[1]
}

// IsPublic returns true if this Tx is planet-public (unencrypted).
func (tx *TxEnvelope) IsPublic() bool {
	return tx.Epoch_0 == 0 && tx.Epoch_1 == 0
}

// PlanetEpochID returns the planet epoch UID recorded in this envelope.
// For channel-encrypted TxMsgs, this is the planet epoch active at seal time.
// For planet-encrypted TxMsgs (or unset), returns zero.
func (tx *TxEnvelope) PlanetEpochID() tag.UID {
	return tag.UID{tx.PlanetEpoch_0, tx.PlanetEpoch_1}
}

// SetPlanetEpochID records the planet epoch UID in the envelope (for channel TxMsgs).
func (tx *TxEnvelope) SetPlanetEpochID(epochID tag.UID) {
	tx.PlanetEpoch_0 = epochID[0]
	tx.PlanetEpoch_1 = epochID[1]
}

func (tx *TxHeader) FromID() tag.UID {
	return tag.UID{tx.FromID_0, tx.FromID_1}
}

func (tx *TxHeader) SetFromID(ID tag.UID) {
	tx.FromID_0 = ID[0]
	tx.FromID_1 = ID[1]
}

func (tx *TxHeader) SetContextID(ID tag.UID) {
	tx.ContextID_0 = ID[0]
	tx.ContextID_1 = ID[1]
}

func (tx *TxHeader) ContextID() tag.UID {
	return tag.UID{tx.ContextID_0, tx.ContextID_1}
}

// OpValueBytes returns the serialized value bytes of op opIndex with the
// leading ValueHeader (flags byte + inline UIDs) skipped — exactly the span
// UnmarshalOpValue decodes.  The ONE authoritative header-skip; the returned
// slice aliases tx.DataStore, so callers that retain it must copy.
func (tx *TxMsg) OpValueBytes(opIndex int) ([]byte, error) {
	if opIndex < 0 || opIndex >= len(tx.Ops) {
		return nil, status.ErrMalformedTx
	}
	op := tx.Ops[opIndex]
	ofs := op.DataOfs
	end := ofs + op.DataLen
	if op.DataLen < 1 || ofs > end || end > uint64(len(tx.DataStore)) {
		return nil, status.ErrBadTxOp
	}

	// skip value header and inline UIDs
	UIDs := tx.DataStore[ofs]
	ofs += 1
	for i := range 4 { // lower nibble specifies inline UIDs
		if (UIDs & (1 << i)) != 0 {
			ofs += tag.UID_Size
		}
	}
	if ofs > end {
		return nil, status.ErrBadTxOp
	}
	return tx.DataStore[ofs:end], nil
}

func (tx *TxMsg) UnmarshalOpValue(opIndex int, out proto.Message) error {
	span, err := tx.OpValueBytes(opIndex)
	if err != nil {
		return err
	}
	return proto.Unmarshal(span, out)
}

func (tx *TxMsg) ExtractValue(attrID, itemID tag.UID, dst proto.Message) error {
	for i, op := range tx.Ops {
		if op.Addr.AttrID == attrID && op.Addr.ItemID == itemID {
			return tx.UnmarshalOpValue(i, dst)
		}
	}
	return status.ErrAttrNotFound
}

func (tx *TxMsg) LoadValue(want *tag.Address, dst proto.Message) error {
	tx.Normalize(false)

	if want.ItemID.IsWildcard() {
		for i := range tx.Ops {
			addr := &tx.Ops[i].Addr
			if addr.NodeID == want.NodeID && addr.AttrID == want.AttrID {
				return tx.UnmarshalOpValue(i, dst)
			}
		}
		return status.ErrAttrNotFound
	}

	N := len(tx.Ops)
	idx, _ := sort.Find(N, func(i int) int {
		return tx.Ops[i].Addr.CompareElementID(want)
	})
	if idx >= N {
		return status.ErrAttrNotFound
	}

	// check we have a match but ignore EditID
	elemID := tx.Ops[idx].Addr.ElementLSM()
	wantID := want.ElementLSM()
	if elemID != wantID {
		return status.ErrAttrNotFound
	}

	return tx.UnmarshalOpValue(idx, dst)
}

// Normalize validates and canonicalizes a TxMsg prior to handling — the
// session/authoring rail's gate (peer-wire ingest never calls it; the
// materializer validates per op, order-free).  After a nil return:
//
//  1. every op's Addr.EditID is set;
//  2. tx.Ops is strictly ascending by full Address — a duplicate full address
//     rejects: within one authored tx every op shares EditID = TxID, so two
//     writes to one element would fold as same-identity divergent bytes, an
//     arrival-order hazard (AOM SD-edit-resolution.md §6.2);
//  3. every op's value span [DataOfs, DataOfs+DataLen) lies inside DataStore
//     with no uint64 wrap, so no read site can address outside it;
//  4. Normalized is process-local: every decode exit resets it, so a wire
//     peer can never assert pre-normalization.
//
// Out of scope, enforced at their own doors: value-header admission (commit
// intake + the materializer, AOM SD-edit-resolution.md §6.1), signature/AEAD
// verification (OpenTx), size ceilings, and Tag syntax (checkTagSyntax).
func (tx *TxMsg) Normalize(force bool) error {
	if !force && tx.Normalized {
		return nil
	}
	storeLen := uint64(len(tx.DataStore))
	for i := range tx.Ops {
		op := &tx.Ops[i]
		if op.Addr.EditID.IsNil() {
			return status.ErrBadTxEdit
		}
		spanEnd := op.DataOfs + op.DataLen
		if spanEnd < op.DataOfs || spanEnd > storeLen {
			return status.ErrBadTxOp
		}
	}
	sort.Slice(tx.Ops, func(i, j int) bool {
		return tx.Ops[i].Addr.Compare(&tx.Ops[j].Addr) < 0
	})
	for i := 1; i < len(tx.Ops); i++ {
		if tx.Ops[i-1].Addr.Compare(&tx.Ops[i].Addr) == 0 {
			return status.ErrBadTxOp
		}
	}

	tx.Normalized = true
	return nil
}

func (tx *TxMsg) Upsert(nodeID, attrID, itemID tag.UID, val proto.Message) error {
	return tx.UpsertFrom(nodeID, attrID, itemID, tag.UID{}, val)
}

// UpsertFrom is Upsert with an explicit lineage base: baseEdit is the EditID of
// the edit the caller actually loaded, carried as the ParentEdit inline UID in
// the op value header (ValueHeaderFlags_ParentEdit — AOM SD-edit-resolution.md §6.1).
// The commit door admits ParentEdit exactly on attrs registered RetainEdits > 1.
// A nil baseEdit is a deliberate unparented (wildcard) write — plain Upsert.
func (tx *TxMsg) UpsertFrom(nodeID, attrID, itemID, baseEdit tag.UID, val proto.Message) error {
	op := TxOp{
		Flags: TxOpFlags_Upsert,
	}
	op.Addr.NodeID = nodeID
	op.Addr.AttrID = attrID
	op.Addr.ItemID = itemID

	return tx.marshalOp(&op, baseEdit, val)
}

func (tx *TxMsg) Delete(elemID tag.ElementID, val proto.Message) error {
	op := TxOp{
		Flags: TxOpFlags_Delete,
		Addr: tag.Address{
			ElementID: elemID,
		},
	}
	return tx.MarshalOp(&op, val)
}

// MarshalOp marshals and appends a TxOp and optional value to the given Tx's data store.
//
// On success:
//   - TxMsg.DataStore is appended with the marshaled value
//   - TxOp.DataOfs and TxOp.DataLen updated
//   - TxOp is appended to TxMsg.Ops
func (tx *TxMsg) MarshalOp(op *TxOp, val proto.Message) error {
	return tx.marshalOp(op, tag.UID{}, val)
}

// marshalOp is the one authoritative op value framing site: the value header
// (flags byte + inline UIDs in ascending flag-bit order) followed by the
// marshaled value.  A set baseEdit frames the ParentEdit inline UID
// (ValueHeaderFlags_ParentEdit — AOM SD-edit-resolution.md §6.1).
func (tx *TxMsg) marshalOp(op *TxOp, baseEdit tag.UID, val proto.Message) error {

	// EditID == TxID on every write (AOM SD-edit-resolution.md §6.1); in-memory /
	// cabinet-key identity only — the op wire does not carry it.
	op.Addr.EditID = tx.TxID()

	// START
	ds := tx.DataStore
	startOfs := len(ds)

	// VALUE HEADER
	headerFlags := ValueHeaderFlags_FromID
	if baseEdit.IsSet() {
		headerFlags |= ValueHeaderFlags_ParentEdit // ParentEdit (§6.1)
	}
	ds = append(ds, byte(headerFlags))
	ds = binary.BigEndian.AppendUint64(ds, tx.FromID_0)
	ds = binary.BigEndian.AppendUint64(ds, tx.FromID_1)
	if baseEdit.IsSet() {
		ds = baseEdit.AppendTo(ds)
	}

	// VALUE CONTENT
	if val != nil {
		if err := checkTagSyntax(val); err != nil {
			return err
		}
		var err error
		ds, err = data.MarshalTo(ds, val)
		if err != nil {
			return err
		}
	}

	// END
	op.DataLen = uint64(len(ds) - startOfs)
	op.DataOfs = uint64(startOfs)
	tx.DataStore = ds
	tx.Ops = append(tx.Ops, *op)
	tx.Normalized = false

	return nil
}

// checkTagSyntax admits a Tag/Tags op value at the authoring door — the last point at which
// an op value is still a typed proto.  Past MarshalOp a tx carries opaque bytes and there is
// no attr→type resolution, so the downstream MaxTxMsgSize gate can weigh a tx but cannot see
// a Tag inside it; siting the rule here keeps the refusal on the producer's own stack, ahead
// of seal and journal.  A non-Tag value costs one type switch.  Tag.Validate owns the rule.
func checkTagSyntax(val proto.Message) error {
	switch leaf := val.(type) {
	case *Tag:
		return leaf.Validate()
	case *Tags:
		return leaf.Validate()
	}
	return nil
}

// MarshalOpAndData marshals a TxOp and its raw value (value header then value content)
// Used for low-level handling and should be used with care.
func (tx *TxMsg) MarshalOpAndData(op *TxOp, opValue []byte) {
	op.DataOfs = uint64(len(tx.DataStore))
	op.DataLen = uint64(len(opValue))
	tx.DataStore = append(tx.DataStore, opValue...)
	tx.Ops = append(tx.Ops, *op)
	tx.Normalized = false
}

func ReadTxMsg(stream io.Reader) (*TxMsg, error) {
	readBytes := func(dst []byte) error {
		for L := 0; L < len(dst); {
			n, err := stream.Read(dst[L:])
			if err != nil {
				return err
			}
			L += n
		}
		return nil
	}

	var preamble TxPreamble
	if err := readBytes(preamble[:]); err != nil {
		return nil, err
	}

	if string(preamble[:4]) != TxPreambleSignature {
		return nil, status.ErrMalformedTx
	}

	tx := TxNew()
	headLen := preamble.TxHeadLen()
	dataLen := preamble.TxDataLen()

	// Use tx.DataStore as a temp store the tx header for unmarshalling, containing TxEnvelope and TxOps.
	{
		needSz := max(headLen, dataLen)
		if cap(tx.DataStore) < needSz {
			tx.DataStore = make([]byte, max(needSz, 2048))
		}

		buf := tx.DataStore[:headLen-int(TxPreambleSize)]
		if err := readBytes(buf); err != nil {
			return nil, err
		}
		if err := tx.UnmarshalHead(buf); err != nil {
			return nil, err
		}
	}

	// Read tx data store -- used for on-demand Value unmarshalling
	tx.DataStore = tx.DataStore[:dataLen]
	if err := readBytes(tx.DataStore); err != nil {
		return nil, err
	}

	tx.reconstructEditIDs()
	return tx, nil
}

// CeilingSize returns the ceiling byte size of this TxMsg as a serialized buffer.
func (tx *TxMsg) CeilingSize() int64 {
	const (
		txBaseSize = int(TxPreambleSize) +
			int(unsafe.Sizeof(TxEnvelope{})) +
			int(unsafe.Sizeof(TxHeader{}))
		txOpSize = int(unsafe.Sizeof(TxOp{}))
	)
	sz := txBaseSize + len(tx.DataStore)
	sz += len(tx.Ops) * txOpSize
	return int64(sz)
}

func (tx *TxMsg) MarshalToWriter(scrap *[]byte, w io.Writer) (err error) {
	writeBytes := func(src []byte) error {
		for L := 0; L < len(src); {
			n, err := w.Write(src[L:])
			if err != nil {
				return err
			}
			L += n
		}
		return nil
	}

	tx.MarshalHeadAndOps(scrap)
	if err = writeBytes(*scrap); err != nil {
		return
	}
	if err = writeBytes(tx.DataStore); err != nil {
		return
	}
	return
}

func (tx *TxMsg) MarshalToBuffer(dst *[]byte) {
	tx.MarshalHeadAndOps(dst)
	*dst = append(*dst, tx.DataStore...)
}

func (tx *TxMsg) MarshalHeadAndOps(dst *[]byte) {
	buf := *dst
	if cap(buf) < 300 {
		buf = make([]byte, 2048)
	}

	headAndOps := tx.MarshalHead(buf[:TxPreambleSize])

	head := headAndOps[:TxPreambleSize]
	copy(head[:4], TxPreambleSignature)

	binary.BigEndian.PutUint32(head[4:8], uint32(len(headAndOps)))
	binary.BigEndian.PutUint32(head[8:12], uint32(len(tx.DataStore)))

	*dst = headAndOps
}

func (tx *TxMsg) MarshalHead(dst []byte) []byte {
	dst, _ = writePb(dst, &tx.TxEnvelope) // write TxEnvelope uvarint & data
	tx.cryptOfs = uint64(len(dst))        // store TxHeader start (encrypt begins here)
	dst, _ = writePb(dst, &tx.TxHeader)   // write TxHeader uvarint & data
	return appendOps(dst, tx.Ops)
}

// appendOps appends the ops section — a fixed u32 BE byte-length prefix
// followed by the delta-compressed ops (flags byte, Logical / DataOfs /
// DataLen / reserved-skip uvarints, hasFields mask, changed 8-byte fields).
// The ONE authoritative encode site; MarshalHead and marshalPayload call it.
// The length prefix self-delimits the section: readers slice it exactly and
// skip it in O(1) — no op count rides the wire.
func appendOps(dst []byte, ops []TxOp) []byte {
	lenOfs := len(dst)
	dst = append(dst, 0, 0, 0, 0) // u32 ops-length, backfilled below

	var (
		opPrv [TxField_MaxFields]uint64
		opCur [TxField_MaxFields]uint64
	)
	for _, op := range ops {
		dst = append(dst, byte(op.Flags))
		dst = binary.AppendUvarint(dst, op.Logical)
		dst = binary.AppendUvarint(dst, op.DataOfs)
		dst = binary.AppendUvarint(dst, op.DataLen)
		dst = binary.AppendUvarint(dst, 0) // skip bytes (future use)

		// detect repeated fields and write only what changes (with corresponding flags)
		opCur[TxField_ItemID_0] = op.Addr.ItemID[0]
		opCur[TxField_ItemID_1] = op.Addr.ItemID[1]

		opCur[TxField_AttrID_0] = op.Addr.AttrID[0]
		opCur[TxField_AttrID_1] = op.Addr.AttrID[1]

		opCur[TxField_NodeID_0] = op.Addr.NodeID[0]
		opCur[TxField_NodeID_1] = op.Addr.NodeID[1]

		hasFields := uint64(0)
		for i, fi := range opCur {
			if fi != opPrv[i] {
				hasFields |= (1 << i)
			}
		}

		dst = binary.AppendUvarint(dst, hasFields)
		for i, fi := range opCur {
			if hasFields&(1<<i) != 0 {
				dst = binary.BigEndian.AppendUint64(dst, fi)
			}
		}

		opPrv = opCur // current becomes previous
	}

	binary.BigEndian.PutUint32(dst[lenOfs:lenOfs+4], uint32(len(dst)-lenOfs-4))
	return dst
}

// readOpsSection slices the u32-length-prefixed ops span out of src at *pos,
// advancing *pos past it, then parses every op in the span into tx.Ops.
func readOpsSection(tx *TxMsg, src []byte, pos *int) error {
	p := *pos
	if p+4 > len(src) {
		return status.ErrMalformedTx
	}
	opsLen := int(binary.BigEndian.Uint32(src[p : p+4]))
	p += 4
	if opsLen > len(src)-p {
		return status.ErrMalformedTx
	}
	if err := readOps(tx, src[p:p+opsLen]); err != nil {
		return err
	}
	*pos = p + opsLen
	return nil
}

// readOps parses delta-compressed ops from src (exactly one ops span, sliced
// by its length prefix) into tx.Ops until src is exhausted — the ONE
// authoritative decode walk; every parse path resolves op boundaries here.
func readOps(tx *TxMsg, src []byte) error {
	p := 0
	var opCur [TxField_MaxFields]uint64

	for p < len(src) {
		var op TxOp
		var n int

		op.Flags = TxOpFlags(src[p])
		p++

		if op.Logical, n = binary.Uvarint(src[p:]); n <= 0 {
			return status.ErrMalformedTx
		}
		p += n

		if op.DataOfs, n = binary.Uvarint(src[p:]); n <= 0 {
			return status.ErrMalformedTx
		}
		p += n

		if op.DataLen, n = binary.Uvarint(src[p:]); n <= 0 {
			return status.ErrMalformedTx
		}
		p += n

		var skip uint64
		if skip, n = binary.Uvarint(src[p:]); n <= 0 {
			return status.ErrMalformedTx
		}
		p += n + int(skip)
		if p > len(src) {
			return status.ErrMalformedTx
		}

		var hasFields uint64
		if hasFields, n = binary.Uvarint(src[p:]); n <= 0 {
			return status.ErrMalformedTx
		}
		p += n

		for j := range int(TxField_MaxFields) {
			if hasFields&(1<<j) != 0 {
				if p+8 > len(src) {
					return status.ErrMalformedTx
				}
				opCur[j] = binary.BigEndian.Uint64(src[p:])
				p += 8
			}
		}

		op.Addr.ItemID[0] = opCur[TxField_ItemID_0]
		op.Addr.ItemID[1] = opCur[TxField_ItemID_1]

		op.Addr.AttrID[0] = opCur[TxField_AttrID_0]
		op.Addr.AttrID[1] = opCur[TxField_AttrID_1]

		op.Addr.NodeID[0] = opCur[TxField_NodeID_0]
		op.Addr.NodeID[1] = opCur[TxField_NodeID_1]

		tx.Ops = append(tx.Ops, op)
	}
	return nil
}

// reconstructEditIDs restores each op's identity after decode — the ONE
// authoritative site (AOM SD-edit-resolution.md §6.1): the envelope TxID, unless the
// op's value header carries an authoring TxID (ValueHeaderFlags_TxID, stamped
// at materialize so served ops survive session-tx re-bundling).  Called only
// at decode exits where the DataStore is populated; nothing downstream ever
// derives identity.
func (tx *TxMsg) reconstructEditIDs() {
	txID := tx.TxID()
	for i := range tx.Ops {
		op := &tx.Ops[i]
		op.Addr.EditID = txID

		if op.DataLen == 0 || op.DataOfs >= uint64(len(tx.DataStore)) {
			continue
		}
		headerFlags := ValueHeaderFlags(tx.DataStore[op.DataOfs])
		if headerFlags&ValueHeaderFlags_TxID == 0 {
			continue
		}
		txIDOfs := op.DataOfs + 1 // inline UIDs follow in ascending flag-bit order
		if headerFlags&ValueHeaderFlags_FromID != 0 {
			txIDOfs += uint64(tag.UID_Size)
		}
		if txIDOfs+uint64(tag.UID_Size) > op.DataOfs+op.DataLen || txIDOfs+uint64(tag.UID_Size) > uint64(len(tx.DataStore)) {
			continue // malformed header: hold the envelope identity; ingest rejects upstream
		}
		op.Addr.EditID[0] = binary.BigEndian.Uint64(tx.DataStore[txIDOfs:])
		op.Addr.EditID[1] = binary.BigEndian.Uint64(tx.DataStore[txIDOfs+8:])
	}
}

func (tx *TxMsg) UnmarshalHead(src []byte) error {
	p := 0

	// TxEnvelope
	tx.TxEnvelope = TxEnvelope{}
	if err := readPb(src, &p, &tx.TxEnvelope); err != nil {
		return err
	}

	tx.TxHeader = TxHeader{}
	if err := readPb(src, &p, &tx.TxHeader); err != nil {
		return err
	}

	if err := readOpsSection(tx, src, &p); err != nil {
		return err
	}

	// ensure we renormalize later
	tx.Normalized = false

	return nil
}

// CryptoProvider supplies the cryptographic operations needed to seal (encrypt+sign) and open (verify+decrypt) TxMsgs.
// Implemented by the vault/host layer using safe.Enclave and safe.Kit.
//
// Methods that accept *TxEnvelope use it to determine the encryption context:
//   - Planet-level TxMsgs: Epoch is the planet epoch; PlanetEpoch is zero.
//   - Channel-level TxMsgs: Epoch is the channel epoch; PlanetEpoch records the planet epoch
//     active at seal time.  Effective keys are derived per role (see safe.KeyRole):
//     content_key = HKDF(node_content_key || planet_epoch_key, "content")
//     proof_key   = HKDF(node_write_seed || planet_epoch_key, "member-proof")
//
// If the required epoch key is not available, methods return status.ErrEpochKeyNotFound.
// Callers should retain the TxMsg and retry when the key arrives.
type CryptoProvider interface {

	// SignatureSize returns the fixed byte length of signatures produced by this provider.
	SignatureSize() int

	// HashDigest computes a cryptographic hash of the given data segments.
	HashDigest(parts ...[]byte) ([32]byte, error)

	// SignDigest produces a signature of the given digest using the author's signing key.
	SignDigest(digest []byte) ([]byte, error)

	// VerifyDigest checks a signature against the digest using the given public key and CryptoKit.
	VerifyDigest(sig []byte, digest []byte, signerPubKey []byte, cryptoKit safe.CryptoKitID) error

	// EncryptPayload encrypts plaintext using the epoch key(s) from the envelope.
	// Returns nil, nil if no encryption is needed (planet-public).
	EncryptPayload(plaintext []byte, env *TxEnvelope) ([]byte, error)

	// DecryptPayload decrypts ciphertext using the epoch key(s) from the envelope.
	// Returns nil, nil if the TxMsg is planet-public (Epoch is nil).
	DecryptPayload(ciphertext []byte, env *TxEnvelope) ([]byte, error)

	// ComputeMemberProof generates HMAC(proof_key, txID) for relay verification.
	// proof_key = HKDF(epoch_key, "member-proof")
	// Returns nil, nil if the TxMsg is planet-public (no epoch).
	ComputeMemberProof(txID []byte, env *TxEnvelope) ([]byte, error)

	// VerifyMemberProof checks that a MemberProof is valid for the given TxID and epoch.
	// Returns nil if the TxMsg is planet-public (no epoch).
	VerifyMemberProof(proof, txID []byte, env *TxEnvelope) error
}

// SealTx marshals, encrypts, and signs a TxMsg producing a complete wire-format byte slice.
//
// One TxMsg = one encryption context: TxEnvelope.Epoch selects a single epoch key.
// All TxOp(s) must belong to the this same encryption domain.
// If the epoch is set, a MemberProof (HMAC over TxID using a derived proof key) is attached for relay verification.
//
// Wire layout:
//
//	Preamble (16B) | TxEnvelope (varint-prefixed) | Payload (encrypted or plaintext) [| DataStore] | Signature
//
// Signature length is stored in preamble[12:14] (uint16 BE). The signature is the trailing bytes of the wire.
//
// If crypto is nil, the TxMsg is marshaled without encryption or signing (local session use).
func SealTx(tx *TxMsg, crypto CryptoProvider, dst *[]byte) error {
	if crypto == nil {
		// No crypto — standard marshal (local session traffic)
		tx.MarshalHeadAndOps(dst)
		return nil
	}

	buf := *dst
	if cap(buf) < 2048 {
		buf = make([]byte, 2048)
	}

	// --- Marshal the payload (TxHeader + ops section) without preamble or envelope ---
	payload := marshalPayload(tx, nil)

	// --- Encrypt payload if epoch is set (private planet/channel) ---
	isPublic := tx.TxEnvelope.IsPublic()
	var wirePayload []byte
	if isPublic {
		wirePayload = payload
	} else {
		// Combine payload + DataStore for encryption (they are a single encrypted blob)
		plaintext := append(payload, tx.DataStore...)
		encrypted, err := crypto.EncryptPayload(plaintext, &tx.TxEnvelope)
		if err != nil {
			return err
		}
		wirePayload = encrypted

		// Compute MemberProof for relay verification (HMAC of proof_key over TxID)
		proof, err := crypto.ComputeMemberProof(tx.TxEnvelope.MemberProofInput(), &tx.TxEnvelope)
		if err != nil {
			return err
		}
		tx.TxEnvelope.MemberProof = proof
	}

	// --- Build the wire buffer: Preamble | Envelope | Payload [| DataStore] | Signature ---

	// Marshal the envelope (MemberProof, when sealed, is set above)
	envBuf, _ := writePb(nil, &tx.TxEnvelope)

	buf = buf[:TxPreambleSize]
	copy(buf[:4], TxPreambleSignature)
	buf = append(buf, envBuf...)
	buf = append(buf, wirePayload...)
	if isPublic {
		buf = append(buf, tx.DataStore...)
	}

	// Preamble size fields
	if isPublic {
		binary.BigEndian.PutUint32(buf[4:8], uint32(int(TxPreambleSize)+len(envBuf)+len(payload)))
		binary.BigEndian.PutUint32(buf[8:12], uint32(len(tx.DataStore)))
	} else {
		binary.BigEndian.PutUint32(buf[4:8], uint32(int(TxPreambleSize)+len(envBuf)+len(wirePayload)))
		binary.BigEndian.PutUint32(buf[8:12], 0) // DataStore is inside encrypted payload
	}

	sigSize := crypto.SignatureSize()
	binary.BigEndian.PutUint16(buf[12:14], uint16(sigSize))

	// --- Sign: domain-separated digest over the wire before signature → append ---
	// safe.SigningParts frames the wire under SigningDomain_TxAuthor — the same
	// segments SigningDigest hashes — so this seal is byte-identical to what
	// TxSignedDigest derives on the verify side.
	framed, err := safe.SigningParts(safe.SigningDomain_TxAuthor, buf)
	if err != nil {
		return err
	}
	digest, err := crypto.HashDigest(framed...)
	if err != nil {
		return err
	}
	sig, err := crypto.SignDigest(digest[:])
	if err != nil {
		return err
	}
	buf = append(buf, sig...)

	*dst = buf
	return nil
}

// TxSigOffset returns the byte offset in a sealed TxMsg wire image where the trailing author
// signature begins, validating the preamble[12:14] signature-length contract (the signature is
// the trailing bytes of the wire).  This is the single authoritative site for that wire-layout
// rule; TxSignedDigest, the planet-public intake guard, and the deferred author-verify path all
// resolve the signature boundary through it.
func TxSigOffset(raw []byte) (sigOfs int, err error) {
	if len(raw) < int(TxPreambleSize) {
		return 0, status.ErrMalformedTx
	}
	sigLen := int(binary.BigEndian.Uint16(raw[12:14]))
	if sigLen == 0 || sigLen > len(raw)-int(TxPreambleSize) {
		return 0, status.Code_BadRequest.Error("malformed signature length")
	}
	return len(raw) - sigLen, nil
}

// TxSignedDigest parses a sealed TxMsg wire image and returns the domain-separated
// digest a verifier checks — SigningDomain_TxAuthor bound into hashKitID run over
// the bytes preceding the signature — together with the trailing signature bytes.
// Verifiers pass these to their chosen backend (safe.VerifySignature, a
// CryptoProvider's VerifyDigest, …); the parse + digest live in one place so the
// wire contract is never re-implemented per caller (SealTx binds the same domain).
func TxSignedDigest(raw []byte, hashKitID safe.HashKitID) (digest, sig []byte, err error) {
	sigOfs, err := TxSigOffset(raw)
	if err != nil {
		return nil, nil, err
	}
	digest, err = safe.SigningDigest(hashKitID, safe.SigningDomain_TxAuthor, raw[:sigOfs])
	if err != nil {
		return nil, nil, err
	}
	return digest, raw[sigOfs:], nil
}

// OpenTx verifies the signature and decrypts a sealed wire-format TxMsg.
// signerPubKey and signerCryptoKit are the author's signing public key and CryptoKit
// (looked up externally from the MemberEpoch via TxHeader.FromID).
//
// If crypto is nil, the buffer is unmarshaled without verification or decryption (local session use).
func OpenTx(wire []byte, crypto CryptoProvider, signerPubKey []byte, signerCryptoKit safe.CryptoKitID) (*TxMsg, error) {
	return openTx(wire, crypto, signerPubKey, signerCryptoKit, true)
}

// OpenTxSansVerify decrypts a sealed, encrypted wire-format TxMsg to surface its ops
// WITHOUT verifying the author signature.  An encrypted TxMsg carries the author's FromID
// inside the ciphertext, so the signer's public key is unknowable until after decryption —
// a receiver must decrypt first to discover it.  This entrypoint is for a member's
// receive-side scan (e.g. blob-ref discovery): the symmetric AEAD already authenticates the
// payload (a wrong or forged ciphertext fails to open) and the relay's MemberProof gates
// acceptance upstream.  Full author-signature verification, where required, follows once
// FromID resolves to a cached member key.
func OpenTxSansVerify(wire []byte, crypto CryptoProvider) (*TxMsg, error) {
	return openTx(wire, crypto, nil, safe.CryptoKitID{}, false)
}

func openTx(wire []byte, crypto CryptoProvider, signerPubKey []byte, signerCryptoKit safe.CryptoKitID, verifySig bool) (*TxMsg, error) {
	if len(wire) < int(TxPreambleSize) {
		return nil, status.ErrMalformedTx
	}

	// Validate preamble
	if string(wire[:4]) != TxPreambleSignature {
		return nil, status.ErrMalformedTx
	}

	tx := TxNew()

	if crypto == nil {
		// No crypto — standard unmarshal
		headLen := int(binary.BigEndian.Uint32(wire[4:8]))
		dataLen := int(binary.BigEndian.Uint32(wire[8:12]))
		if headLen < int(TxPreambleSize) || headLen > len(wire) || dataLen > len(wire)-headLen {
			return nil, status.ErrMalformedTx
		}
		headBody := wire[TxPreambleSize:headLen]
		if err := tx.UnmarshalHead(headBody); err != nil {
			return nil, err
		}
		if dataLen > 0 {
			tx.DataStore = make([]byte, dataLen)
			copy(tx.DataStore, wire[headLen:headLen+dataLen])
		}
		tx.reconstructEditIDs()
		return tx, nil
	}

	// --- Parse TxEnvelope from the head (in the clear) ---
	headLen := int(binary.BigEndian.Uint32(wire[4:8]))
	dataLen := int(binary.BigEndian.Uint32(wire[8:12]))
	if headLen < int(TxPreambleSize) || headLen > len(wire) || dataLen > len(wire)-headLen {
		return nil, status.ErrMalformedTx
	}
	headBody := wire[TxPreambleSize:headLen]

	// Read just the envelope
	p := 0
	if err := readPb(headBody, &p, &tx.TxEnvelope); err != nil {
		return nil, err
	}

	// --- Verify signature ---
	// The trailing-signature boundary (the preamble[12:14] contract) resolves through the one
	// authoritative site, TxSigOffset.
	sigOfs, err := TxSigOffset(wire)
	if err != nil {
		return nil, err
	}

	// sigOfs bounds the ciphertext for the encrypted branch below and is always needed.
	// The author-signature check itself is skipped on the SansVerify decrypt-read path
	// (the signer's pubkey lives inside the still-sealed payload; AEAD authenticates it).
	if verifySig {
		signedData := wire[:sigOfs]
		sig := wire[sigOfs:]

		framed, err := safe.SigningParts(safe.SigningDomain_TxAuthor, signedData)
		if err != nil {
			return nil, err
		}
		digest, err := crypto.HashDigest(framed...)
		if err != nil {
			return nil, err
		}
		if err := crypto.VerifyDigest(sig, digest[:], signerPubKey, signerCryptoKit); err != nil {
			return nil, err
		}
	}

	// --- Decrypt if needed ---
	isPublic := tx.TxEnvelope.IsPublic()

	if isPublic {
		// Planet-public: payload is plaintext, DataStore is separate.
		payloadAndOps := headBody[p:]

		// Unmarshal TxHeader + ops section from plaintext
		tx.TxHeader = TxHeader{}
		hp := 0
		if err := readPb(payloadAndOps, &hp, &tx.TxHeader); err != nil {
			return nil, err
		}
		if err := readOpsSection(tx, payloadAndOps, &hp); err != nil {
			return nil, err
		}

		// DataStore
		if dataLen > 0 {
			dsStart := headLen
			tx.DataStore = make([]byte, dataLen)
			copy(tx.DataStore, wire[dsStart:dsStart+dataLen])
		}
	} else {
		// Encrypted: payload contains TxHeader + ops section + DataStore.
		// sigLen (→ sigOfs) is an attacker-controlled wire field, and this branch is
		// reached via OpenTxSansVerify with no prior signature check, so bound the
		// ciphertext span before slicing — an out-of-range or inverted span would
		// otherwise panic the receive goroutine.
		encryptedStart := int(TxPreambleSize) + p
		encryptedEnd := int(sigOfs)
		if encryptedEnd > len(wire) || encryptedEnd < encryptedStart {
			return nil, status.ErrMalformedTx
		}
		ciphertext := wire[encryptedStart:encryptedEnd]

		plaintext, err := crypto.DecryptPayload(ciphertext, &tx.TxEnvelope)
		if err != nil {
			return nil, err
		}

		// The plaintext is: TxHeader | ops section (u32-length-prefixed) | DataStore.
		hp := 0
		tx.TxHeader = TxHeader{}
		if err := readPb(plaintext, &hp, &tx.TxHeader); err != nil {
			return nil, err
		}
		if err := readOpsSection(tx, plaintext, &hp); err != nil {
			return nil, err
		}
		if hp < len(plaintext) {
			tx.DataStore = make([]byte, len(plaintext)-hp)
			copy(tx.DataStore, plaintext[hp:])
		}
	}

	tx.Normalized = false
	tx.reconstructEditIDs()
	return tx, nil
}

// ParseTxEnvelope extracts just the TxEnvelope from a sealed wire-format TxMsg
// without verifying, decrypting, or parsing the payload.
//
// This is used by relay vaults and VaultController to inspect cleartext routing
// metadata (PlanetID, Epoch, TxID, MemberProof) without needing the epoch key
// or signer's public key.
func ParseTxEnvelope(wire []byte) (*TxEnvelope, error) {
	if len(wire) < int(TxPreambleSize) {
		return nil, status.ErrMalformedTx
	}

	if string(wire[:4]) != TxPreambleSignature {
		return nil, status.ErrMalformedTx
	}

	env := &TxEnvelope{}
	p := 0
	if err := readPb(wire[TxPreambleSize:], &p, env); err != nil {
		return nil, err
	}
	return env, nil
}

// marshalPayload marshals TxHeader + ops section (the encrypted portion) without preamble or envelope.
func marshalPayload(tx *TxMsg, dst []byte) []byte {
	dst, _ = writePb(dst, &tx.TxHeader)
	return appendOps(dst, tx.Ops)
}

// Marshals a proto.Message with a Uvarint length prefix
func writePb(dst []byte, pb proto.Message) ([]byte, error) {
	buf, err := data.MarshalTo(nil, pb)
	if err != nil {
		return dst, err
	}
	dst = binary.AppendUvarint(dst, uint64(len(buf)))
	dst = append(dst, buf...)
	return dst, nil
}

// Unmarshals a proto.Message with a Uvarint length prefix
func readPb(src []byte, pos *int, pb proto.Message) error {
	p := *pos
	if p < 0 || p >= len(src) {
		return status.ErrMalformedTx
	}

	byteLen, n := binary.Uvarint(src[p:])
	if n <= 0 {
		return status.ErrMalformedTx
	}
	p += n

	end := p + int(byteLen)
	if end > len(src) {
		return status.ErrMalformedTx
	}

	if err := proto.Unmarshal(src[p:end], pb); err != nil {
		return status.ErrMalformedTx
	}

	*pos = end
	return nil
}
