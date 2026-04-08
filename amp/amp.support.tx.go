package amp

import (
	"encoding/binary"
	"io"
	"sort"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/art-media-platform/amp.SDK/stdlib/data"
	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"google.golang.org/protobuf/proto"
)

const (
	// Signature size appended to sealed TxMsgs (determined by the CryptoKit in use).
	TxSignatureSize = 64
)

// TxDataStore is a message packet sent to / from a client.
// It leads with a fixed-size header (TxPreamble_Size).
type TxDataStore []byte

// TxPreamble is the fixed-size header that leads every TxMsg.
// See comments for Const_TxPreamble_Size.
type TxPreamble [Const_TxPreamble_Size]byte

func (preamble TxPreamble) TxHeadLen() int {
	return int(binary.BigEndian.Uint32(preamble[4:8]))
}

func (preamble TxPreamble) TxDataLen() int {
	return int(binary.BigEndian.Uint32(preamble[8:12]))
}

func TxNew() *TxMsg {
	tx := gTxMsgPool.Get().(*TxMsg)
	tx.Normalized = false
	tx.refCount = 1
	return tx
}

var gTxMsgPool = sync.Pool{
	New: func() any {
		return &TxMsg{}
	},
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

func (tx *TxMsg) AddRef() {
	atomic.AddInt32(&tx.refCount, 1)
}

func (tx *TxMsg) AddRefs(delta int) {
	if delta == 0 {
		return
	}
	if delta < 0 || delta > 0x7FFFFFFF {
		panic("AddRefs: invalid delta")
	}
	atomic.AddInt32(&tx.refCount, int32(delta))
}

func (tx *TxMsg) ReleaseRef() {
	// TODO systematic ReleaseRef() makeover in amp.planet

	// newCount := atomic.AddInt32(&tx.refCount, -1)
	// if newCount != 0 {
	// 	return
	// }

	// *tx = TxMsg{
	// 	Ops:       tx.Ops[:0],
	// 	DataStore: tx.DataStore[:0],
	// }
	// gTxMsgPool.Put(tx)
}

func (tx *TxMsg) UnmarshalOpValue(opIndex int, out proto.Message) error {
	if opIndex < 0 || opIndex >= len(tx.Ops) {
		return status.ErrMalformedTx
	}
	op := tx.Ops[opIndex]
	ofs := op.DataOfs
	end := ofs + op.DataLen
	if op.DataLen < 1 || ofs > end || end > uint64(len(tx.DataStore)) {
		return status.ErrBadTxOp
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
		return status.ErrBadTxOp
	}
	span := tx.DataStore[ofs:end]
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
		panic("TODO") // add ItemID wildcard support; replicate code in amp.3D.client
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
	tx.OpCount++
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
	tx.OpCount++
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

	marker := uint32(preamble[0])<<16 | uint32(preamble[1])<<8 | uint32(preamble[2])
	if marker != uint32(Const_TxPreamble_Marker) {
		return nil, status.ErrMalformedTx
	}
	if preamble[3] < byte(Const_TxPreamble_Version) {
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

		buf := tx.DataStore[:headLen-int(Const_TxPreamble_Size)]
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
		txBaseSize = int(Const_TxPreamble_Size) +
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

	headAndOps := tx.MarshalHead(buf[:Const_TxPreamble_Size])

	head := headAndOps[:Const_TxPreamble_Size]
	head[0] = byte((Const_TxPreamble_Marker >> 16) & 0xFF)
	head[1] = byte((Const_TxPreamble_Marker >> 8) & 0xFF)
	head[2] = byte((Const_TxPreamble_Marker >> 0) & 0xFF)
	head[3] = byte(Const_TxPreamble_Version)

	binary.BigEndian.PutUint32(head[4:8], uint32(len(headAndOps)))
	binary.BigEndian.PutUint32(head[8:12], uint32(len(tx.DataStore)))

	*dst = headAndOps
}

func (tx *TxMsg) MarshalHead(dst []byte) []byte {

	tx.OpCount = uint64(len(tx.Ops))
	tx.TxEnvelope.HeaderOffset = 0        // byte skip before TxHeader
	dst, _ = writePb(dst, &tx.TxEnvelope) // write TxEnvelope uvarint & data
	tx.cryptOfs = uint64(len(dst))        // store TxHeader start (encrypt begins here)
	dst, _ = writePb(dst, &tx.TxHeader)   // write TxHeader uvarint & data

	var (
		op_prv [TxField_MaxFields]uint64
		op_cur [TxField_MaxFields]uint64
	)

	for _, op := range tx.Ops {
		dst = append(dst, byte(op.Flags))
		dst = binary.AppendUvarint(dst, op.Citation)
		dst = binary.AppendUvarint(dst, op.DataOfs)
		dst = binary.AppendUvarint(dst, op.DataLen)
		dst = binary.AppendUvarint(dst, 0) // skip bytes (future use)

		// detect repeated fields and write only what changes (with corresponding flags)
		{
			op_cur[TxField_NodeID_0] = op.Addr.NodeID[0]
			op_cur[TxField_NodeID_1] = op.Addr.NodeID[1]

			op_cur[TxField_AttrID_0] = op.Addr.AttrID[0]
			op_cur[TxField_AttrID_1] = op.Addr.AttrID[1]

			op_cur[TxField_ItemID_0] = op.Addr.ItemID[0]
			op_cur[TxField_ItemID_1] = op.Addr.ItemID[1]

			op_cur[TxField_EditID_0] = op.Addr.EditID[0]
			op_cur[TxField_EditID_1] = op.Addr.EditID[1]

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
	}

	return dst
}

func (tx *TxMsg) UnmarshalHead(src []byte) error {
	p := 0

	// TxEnvelope
	tx.TxEnvelope = TxEnvelope{}
	if err := readPb(src, &p, &tx.TxEnvelope); err != nil {
		return err
	}

	p += int(tx.TxEnvelope.HeaderOffset)

	tx.TxHeader = TxHeader{}
	if err := readPb(src, &p, &tx.TxHeader); err != nil {
		return err
	}

	var (
		op_cur [TxField_MaxFields]uint64
	)

	for i := uint64(0); i < tx.OpCount; i++ {
		var op TxOp
		var n int

		// OpFlags
		op.Flags = TxOpFlags(src[p])
		p += 1

		// Citation
		if op.Citation, n = binary.Uvarint(src[p:]); n <= 0 {
			return status.ErrMalformedTx
		}
		p += n

		// DataOfs
		if op.DataOfs, n = binary.Uvarint(src[p:]); n <= 0 {
			return status.ErrMalformedTx
		}
		p += n

		// DataLen
		if op.DataLen, n = binary.Uvarint(src[p:]); n <= 0 {
			return status.ErrMalformedTx
		}
		p += n

		// reserved / future use
		var skip uint64
		if skip, n = binary.Uvarint(src[p:]); n <= 0 {
			return status.ErrMalformedTx
		}
		p += n + int(skip)
		if p > len(src) {
			return status.ErrMalformedTx
		}

		// hasFields
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

		op.Addr.NodeID[0] = op_cur[TxField_NodeID_0]
		op.Addr.NodeID[1] = op_cur[TxField_NodeID_1]

		op.Addr.AttrID[0] = op_cur[TxField_AttrID_0]
		op.Addr.AttrID[1] = op_cur[TxField_AttrID_1]

		op.Addr.ItemID[0] = op_cur[TxField_ItemID_0]
		op.Addr.ItemID[1] = op_cur[TxField_ItemID_1]

		op.Addr.EditID[0] = op_cur[TxField_EditID_0]
		op.Addr.EditID[1] = op_cur[TxField_EditID_1]

		tx.Ops = append(tx.Ops, op)
	}

	// ensure we renormalize later
	tx.Normalized = false

	return nil
}

// CryptoProvider supplies the cryptographic operations needed to seal (encrypt+sign) and open (verify+decrypt) TxMsgs.
// Implemented by the vault/host layer using safe.Enclave and safe.CryptoKit.
//
// Methods that accept *TxEnvelope use it to determine the encryption context:
//   - Planet-level TxMsgs: Epoch is the planet epoch; PlanetEpoch is zero.
//   - Channel-level TxMsgs: Epoch is the channel epoch; PlanetEpoch records the planet epoch
//     active at seal time.  The effective key is derived from both:
//     content_key = HKDF(channel_epoch_key || planet_epoch_key, "content")
//
// If the required epoch key is not available, methods return status.ErrEpochKeyNotFound.
// Callers should retain the TxMsg and retry when the key arrives.
type CryptoProvider interface {

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
//	Preamble (16B) | TxEnvelope (varint-prefixed) | Payload (encrypted or plaintext) | DataStore | Signature (64B)
//
// If crypto is nil, the TxMsg is marshaled without encryption or signing (local session use).
func SealTx(tx *TxMsg, crypto CryptoProvider, dst *[]byte) error {
	buf := *dst
	if cap(buf) < 2048 {
		buf = make([]byte, 2048)
	}

	// --- Marshal the payload (TxHeader + TxOps) without preamble or envelope ---
	tx.OpCount = uint64(len(tx.Ops))
	tx.TxEnvelope.HeaderOffset = 0

	// Marshal TxEnvelope to a temp buffer so we can measure it
	envBuf, _ := writePb(nil, &tx.TxEnvelope)

	// Marshal payload: TxHeader + TxOps
	payload := marshalPayload(tx, nil)

	if crypto == nil {
		// No crypto — standard marshal (local session traffic)
		tx.MarshalHeadAndOps(dst)
		return nil
	}

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
	buf = buf[:Const_TxPreamble_Size]

	// Preamble
	buf[0] = byte((Const_TxPreamble_Marker >> 16) & 0xFF)
	buf[1] = byte((Const_TxPreamble_Marker >> 8) & 0xFF)
	buf[2] = byte((Const_TxPreamble_Marker >> 0) & 0xFF)
	buf[3] = byte(Const_TxPreamble_Version)

	// Envelope
	buf = append(buf, envBuf...)

	// Payload (+ DataStore if not encrypted together)
	buf = append(buf, wirePayload...)
	if isPublic {
		buf = append(buf, tx.DataStore...)
	}

	// Fill in preamble sizes now that we know the total
	sigOfs := len(buf)
	tx.TxEnvelope.SignatureOffset = uint64(sigOfs)

	// Re-marshal envelope with SignatureOffset set (it changed)
	envBuf, _ = writePb(nil, &tx.TxEnvelope)
	// Rebuild: we need the envelope to be final before signing
	buf = buf[:Const_TxPreamble_Size]
	buf = append(buf, envBuf...)
	buf = append(buf, wirePayload...)
	if isPublic {
		buf = append(buf, tx.DataStore...)
	}

	// Update preamble size fields
	if isPublic {
		binary.BigEndian.PutUint32(buf[4:8], uint32(int(Const_TxPreamble_Size)+len(envBuf)+len(payload)))
		binary.BigEndian.PutUint32(buf[8:12], uint32(len(tx.DataStore)))
	} else {
		// Encrypted: payload+datastore are combined in wirePayload
		binary.BigEndian.PutUint32(buf[4:8], uint32(int(Const_TxPreamble_Size)+len(envBuf)+len(wirePayload)))
		binary.BigEndian.PutUint32(buf[8:12], 0) // DataStore is inside encrypted payload
	}

	// Update SignatureOffset (now final)
	tx.TxEnvelope.SignatureOffset = uint64(len(buf))

	// Re-marshal envelope one final time with correct SignatureOffset
	envBuf, _ = writePb(nil, &tx.TxEnvelope)
	buf = buf[:Const_TxPreamble_Size]
	buf = append(buf, envBuf...)
	buf = append(buf, wirePayload...)
	if isPublic {
		buf = append(buf, tx.DataStore...)
	}

	// --- Sign: hash(Preamble || Envelope || Payload) then sign ---
	digest, err := crypto.HashDigest(buf)
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

// OpenTx verifies the signature and decrypts a sealed wire-format TxMsg.
// signerPubKey and signerCryptoKit are the author's signing public key and CryptoKit
// (looked up externally from the MemberEpoch via TxHeader.FromID).
//
// If crypto is nil, the buffer is unmarshaled without verification or decryption (local session use).
func OpenTx(wire []byte, crypto CryptoProvider, signerPubKey []byte, signerCryptoKit safe.CryptoKitID) (*TxMsg, error) {
	if len(wire) < int(Const_TxPreamble_Size) {
		return nil, status.ErrMalformedTx
	}

	// Validate preamble
	marker := uint32(wire[0])<<16 | uint32(wire[1])<<8 | uint32(wire[2])
	if marker != uint32(Const_TxPreamble_Marker) {
		return nil, status.ErrMalformedTx
	}
	if wire[3] < byte(Const_TxPreamble_Version) {
		return nil, status.ErrMalformedTx
	}

	tx := TxNew()

	if crypto == nil {
		// No crypto — standard unmarshal
		headLen := int(binary.BigEndian.Uint32(wire[4:8]))
		dataLen := int(binary.BigEndian.Uint32(wire[8:12]))
		headBody := wire[Const_TxPreamble_Size:headLen]
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
	headBody := wire[Const_TxPreamble_Size:headLen]

	// Read just the envelope
	p := 0
	if err := readPb(headBody, &p, &tx.TxEnvelope); err != nil {
		return nil, err
	}

	// --- Verify signature ---
	sigOfs := tx.TxEnvelope.SignatureOffset
	if sigOfs == 0 || int(sigOfs)+TxSignatureSize > len(wire) {
		return nil, status.ErrMalformedTx
	}

	signedData := wire[:sigOfs]
	sig := wire[sigOfs : sigOfs+TxSignatureSize]

	digest, err := crypto.HashDigest(signedData)
	if err != nil {
		return nil, err
	}
	if err := crypto.VerifyDigest(sig, digest[:], signerPubKey, signerCryptoKit); err != nil {
		return nil, err
	}

	// --- Decrypt if needed ---
	isPublic := tx.TxEnvelope.IsPublic()

	if isPublic {
		// Planet-public: payload is plaintext, DataStore is separate
		payloadStart := int(Const_TxPreamble_Size) + p + int(tx.TxEnvelope.HeaderOffset)
		payloadAndOps := headBody[p+int(tx.TxEnvelope.HeaderOffset):]

		// Unmarshal TxHeader + TxOps from plaintext
		tx.TxHeader = TxHeader{}
		hp := 0
		if err := readPb(payloadAndOps, &hp, &tx.TxHeader); err != nil {
			return nil, err
		}
		if err := unmarshalOps(tx, payloadAndOps[hp:]); err != nil {
			return nil, err
		}

		// DataStore
		if dataLen > 0 {
			dsStart := headLen
			_ = payloadStart // used for clarity
			tx.DataStore = make([]byte, dataLen)
			copy(tx.DataStore, wire[dsStart:dsStart+dataLen])
		}
	} else {
		// Encrypted: payload contains TxHeader + TxOps + DataStore
		encryptedStart := int(Const_TxPreamble_Size) + p + int(tx.TxEnvelope.HeaderOffset)
		encryptedEnd := int(sigOfs)
		ciphertext := wire[encryptedStart:encryptedEnd]

		plaintext, err := crypto.DecryptPayload(ciphertext, &tx.TxEnvelope)
		if err != nil {
			return nil, err
		}

		// The plaintext is: marshalPayload output + DataStore
		// We need to split them. The headLen preamble field tells us where ops end.
		// For encrypted mode, dataLen=0 in the preamble and the original headLen covers
		// Preamble + Envelope only. The payload is self-contained.
		//
		// Re-unmarshal from the decrypted plaintext
		hp := 0
		tx.TxHeader = TxHeader{}
		if err := readPb(plaintext, &hp, &tx.TxHeader); err != nil {
			return nil, err
		}

		// Find where ops end — we marshal OpCount ops, then remainder is DataStore
		opsAndData := plaintext[hp:]
		opsEnd, err := skipOps(opsAndData, tx.OpCount)
		if err != nil {
			return nil, err
		}
		if err := unmarshalOps(tx, opsAndData[:opsEnd]); err != nil {
			return nil, err
		}
		if opsEnd < len(opsAndData) {
			tx.DataStore = make([]byte, len(opsAndData)-opsEnd)
			copy(tx.DataStore, opsAndData[opsEnd:])
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
	if len(wire) < int(Const_TxPreamble_Size) {
		return nil, status.ErrMalformedTx
	}

	marker := uint32(wire[0])<<16 | uint32(wire[1])<<8 | uint32(wire[2])
	if marker != uint32(Const_TxPreamble_Marker) {
		return nil, status.ErrMalformedTx
	}

	env := &TxEnvelope{}
	p := 0
	if err := readPb(wire[Const_TxPreamble_Size:], &p, env); err != nil {
		return nil, err
	}
	return env, nil
}

// marshalPayload marshals TxHeader + TxOps (the encrypted portion) without preamble or envelope.
func marshalPayload(tx *TxMsg, dst []byte) []byte {
	dst, _ = writePb(dst, &tx.TxHeader)

	var (
		op_prv [TxField_MaxFields]uint64
		op_cur [TxField_MaxFields]uint64
	)

	for _, op := range tx.Ops {
		dst = append(dst, byte(op.Flags))
		dst = binary.AppendUvarint(dst, op.Citation)
		dst = binary.AppendUvarint(dst, op.DataOfs)
		dst = binary.AppendUvarint(dst, op.DataLen)
		dst = binary.AppendUvarint(dst, 0) // skip bytes (future use)

		op_cur[TxField_NodeID_0] = op.Addr.NodeID[0]
		op_cur[TxField_NodeID_1] = op.Addr.NodeID[1]
		op_cur[TxField_AttrID_0] = op.Addr.AttrID[0]
		op_cur[TxField_AttrID_1] = op.Addr.AttrID[1]
		op_cur[TxField_ItemID_0] = op.Addr.ItemID[0]
		op_cur[TxField_ItemID_1] = op.Addr.ItemID[1]
		op_cur[TxField_EditID_0] = op.Addr.EditID[0]
		op_cur[TxField_EditID_1] = op.Addr.EditID[1]

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
		op_prv = op_cur
	}

	return dst
}

// unmarshalOps reads compressed TxOp fields from src into tx.Ops.
func unmarshalOps(tx *TxMsg, src []byte) error {
	p := 0
	var op_cur [TxField_MaxFields]uint64

	for i := uint64(0); i < tx.OpCount; i++ {
		if p >= len(src) {
			return status.ErrMalformedTx
		}
		var op TxOp
		var n int

		op.Flags = TxOpFlags(src[p])
		p++

		if op.Citation, n = binary.Uvarint(src[p:]); n <= 0 {
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

		op.Addr.NodeID[0] = op_cur[TxField_NodeID_0]
		op.Addr.NodeID[1] = op_cur[TxField_NodeID_1]
		op.Addr.AttrID[0] = op_cur[TxField_AttrID_0]
		op.Addr.AttrID[1] = op_cur[TxField_AttrID_1]
		op.Addr.ItemID[0] = op_cur[TxField_ItemID_0]
		op.Addr.ItemID[1] = op_cur[TxField_ItemID_1]
		op.Addr.EditID[0] = op_cur[TxField_EditID_0]
		op.Addr.EditID[1] = op_cur[TxField_EditID_1]

		tx.Ops = append(tx.Ops, op)
	}

	return nil
}

// skipOps advances past OpCount encoded ops, returning the byte offset where ops end.
func skipOps(src []byte, opCount uint64) (int, error) {
	p := 0
	for i := uint64(0); i < opCount; i++ {
		if p >= len(src) {
			return 0, status.ErrMalformedTx
		}
		p++ // Flags

		var n int
		if _, n = binary.Uvarint(src[p:]); n <= 0 {
			return 0, status.ErrMalformedTx
		}
		p += n // Citation

		if _, n = binary.Uvarint(src[p:]); n <= 0 {
			return 0, status.ErrMalformedTx
		}
		p += n // DataOfs

		if _, n = binary.Uvarint(src[p:]); n <= 0 {
			return 0, status.ErrMalformedTx
		}
		p += n // DataLen

		var skip uint64
		if skip, n = binary.Uvarint(src[p:]); n <= 0 {
			return 0, status.ErrMalformedTx
		}
		p += n + int(skip)
		if p > len(src) {
			return 0, status.ErrMalformedTx
		}

		var hasFields uint64
		if hasFields, n = binary.Uvarint(src[p:]); n <= 0 {
			return 0, status.ErrMalformedTx
		}
		p += n

		for j := range int(TxField_MaxFields) {
			if hasFields&(1<<j) != 0 {
				p += 8
			}
		}
		if p > len(src) {
			return 0, status.ErrMalformedTx
		}
	}
	return p, nil
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
