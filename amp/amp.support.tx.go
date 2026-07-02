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
// It leads with a fixed-size header (TxPreamble_Size).
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

// IsPublic returns true if this Tx is planet-public (unencrypted).
func (tx *TxEnvelope) IsPublic() bool {
	return tx.Epoch == nil || tx.Epoch.IsNil()
}

// PlanetEpochUID returns the planet epoch UID recorded in this envelope.
// For channel-encrypted TxMsgs, this is the planet epoch active at seal time.
// For planet-encrypted TxMsgs (or unset), returns zero.
func (tx *TxEnvelope) PlanetEpochID() tag.UID {
	return tag.UID{tx.PlanetEpoch_0, tx.PlanetEpoch_1}
}

// SetPlanetEpoch records the planet epoch UID in the envelope (for channel TxMsgs).
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

// Normalizes and validates a TxMsg prior to handling.
func (tx *TxMsg) Normalize(force bool) error {
	if !force && tx.Normalized {
		return nil
	}
	for _, op := range tx.Ops {
		if op.Addr.EditID.IsNil() {
			return status.ErrBadTxEdit
		}
	}
	sort.Slice(tx.Ops, func(i, j int) bool {
		return tx.Ops[i].Addr.Compare(&tx.Ops[j].Addr) < 0
	})

	// TODO: validate other parts of TxMsg?

	tx.Normalized = true
	return nil
}

func (tx *TxMsg) Upsert(nodeID, attrID, itemID tag.UID, val proto.Message) error {
	op := TxOp{
		Flags: TxOpFlags_Upsert,
	}
	op.Addr.NodeID = nodeID
	op.Addr.AttrID = attrID
	op.Addr.ItemID = itemID

	return tx.MarshalOp(&op, val)
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

// Marshals and appends a TxOp and optional value to the given Tx's data store.
//
// On success:
//   - TxMsg.DataStore is appended with the marshaled value
//   - TxOp.DataOfs and TxOp.DataLen updated
//   - TxOp is appended to TxMsg.Ops
func (tx *TxMsg) MarshalOp(op *TxOp, val proto.Message) error {

	// Derive EditID from the TxID (matching C# TxMsg.MarshalOp behavior)
	txID := tx.TxID()
	op.Addr.EditID = txID.DeriveID(op.Addr.EditID)

	// START
	ds := tx.DataStore
	startOfs := len(ds)

	// VALUE HEADER
	headerFlags := ValueHeaderFlags_FromID
	ds = append(ds, byte(headerFlags))
	ds = binary.BigEndian.AppendUint64(ds, tx.FromID_0)
	ds = binary.BigEndian.AppendUint64(ds, tx.FromID_1)

	// VALUE CONTENT
	if val != nil {
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

// Marshals a TxOp and it's raw value (value header then value content)
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

	return tx, nil
}

// Returns the ceiling byte size of this TxMsg as a serialized buffer.
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
		op_prv [TxField_MaxFields]uint64
		op_cur [TxField_MaxFields]uint64
	)
	for _, op := range ops {
		dst = append(dst, byte(op.Flags))
		dst = binary.AppendUvarint(dst, op.Logical)
		dst = binary.AppendUvarint(dst, op.DataOfs)
		dst = binary.AppendUvarint(dst, op.DataLen)
		dst = binary.AppendUvarint(dst, 0) // skip bytes (future use)

		// detect repeated fields and write only what changes (with corresponding flags)
		op_cur[TxField_EditID_0] = op.Addr.EditID[0]
		op_cur[TxField_EditID_1] = op.Addr.EditID[1]

		op_cur[TxField_ItemID_0] = op.Addr.ItemID[0]
		op_cur[TxField_ItemID_1] = op.Addr.ItemID[1]

		op_cur[TxField_AttrID_0] = op.Addr.AttrID[0]
		op_cur[TxField_AttrID_1] = op.Addr.AttrID[1]

		op_cur[TxField_NodeID_0] = op.Addr.NodeID[0]
		op_cur[TxField_NodeID_1] = op.Addr.NodeID[1]

		hasFields := uint64(0)
		for i, fi := range op_cur {
			if fi != op_prv[i] {
				hasFields |= (1 << i)
			}
		}

		dst = binary.AppendUvarint(dst, hasFields)
		for i, fi := range op_cur {
			if hasFields&(1<<i) != 0 {
				dst = binary.BigEndian.AppendUint64(dst, fi)
			}
		}

		op_prv = op_cur // current becomes previous
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
	var op_cur [TxField_MaxFields]uint64

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
				op_cur[j] = binary.BigEndian.Uint64(src[p:])
				p += 8
			}
		}

		op.Addr.EditID[0] = op_cur[TxField_EditID_0]
		op.Addr.EditID[1] = op_cur[TxField_EditID_1]

		op.Addr.ItemID[0] = op_cur[TxField_ItemID_0]
		op.Addr.ItemID[1] = op_cur[TxField_ItemID_1]

		op.Addr.AttrID[0] = op_cur[TxField_AttrID_0]
		op.Addr.AttrID[1] = op_cur[TxField_AttrID_1]

		op.Addr.NodeID[0] = op_cur[TxField_NodeID_0]
		op.Addr.NodeID[1] = op_cur[TxField_NodeID_1]

		tx.Ops = append(tx.Ops, op)
	}
	return nil
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
		txIDBytes := make([]byte, 16)
		binary.BigEndian.PutUint64(txIDBytes[0:8], tx.TxEnvelope.TxID_0)
		binary.BigEndian.PutUint64(txIDBytes[8:16], tx.TxEnvelope.TxID_1)
		proof, err := crypto.ComputeMemberProof(txIDBytes, &tx.TxEnvelope)
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
	// The SigningDomain_TxAuthor tag (safe.sign.go) prefixes the hashed bytes so an
	// author seal can never be produced or accepted in another signing context.
	digest, err := crypto.HashDigest(safe.SigningDomainTag(safe.SigningDomain_TxAuthor), buf)
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

		digest, err := crypto.HashDigest(safe.SigningDomainTag(safe.SigningDomain_TxAuthor), signedData)
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
