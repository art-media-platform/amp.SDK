package amp

import (
	"encoding/binary"
	"io"
	"sort"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/art-media-platform/amp.SDK/stdlib/tag"
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

func (tx *TxHeader) SetID(ID tag.UID) {
	tx.TxID_0 = ID[0]
	tx.TxID_1 = ID[1]
}

func (tx *TxHeader) SetContextID(ID tag.UID) {
	tx.ContextID_0 = ID[0]
	tx.ContextID_1 = ID[1]
}

func (tx *TxHeader) ContextID() tag.UID {
	return tag.UID{tx.ContextID_0, tx.ContextID_1}
}

func (tx *TxHeader) SetFromID(ID tag.UID) {
	tx.FromID_0 = ID[0]
	tx.FromID_1 = ID[1]
}

func (tx *TxHeader) FromID() tag.UID {
	return tag.UID{tx.FromID_0, tx.FromID_1}
}

func (tx *TxHeader) ID() tag.UID {
	return tag.UID{tx.TxID_0, tx.TxID_1}
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

func (tx *TxMsg) UnmarshalOpValue(opIndex int, out Value) error {
	if opIndex < 0 || opIndex >= len(tx.Ops) {
		return ErrMalformedTx
	}
	op := tx.Ops[opIndex]
	ofs := op.DataOfs
	end := ofs + op.DataLen
	if op.DataLen < 1 || ofs > end || end > uint64(len(tx.DataStore)) {
		return ErrBadTxOp
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
		return ErrBadTxOp
	}
	span := tx.DataStore[ofs:end]
	return out.Unmarshal(span)
}

func (tx *TxMsg) ExtractValue(attrID, itemID tag.UID, dst Value) error {
	for i, op := range tx.Ops {
		if op.Addr.AttrID == attrID && op.Addr.ItemID == itemID {
			return tx.UnmarshalOpValue(i, dst)
		}
	}
	return ErrAttrNotFound
}

func (tx *TxMsg) LoadValue(want *tag.Address, dst Value) error {
	tx.Normalize(false)

	if want.ItemID.IsWildcard() {
		panic("TODO") // add ItemID wildcard support; replicate code in amp.3D.client
	}

	N := len(tx.Ops)
	idx, _ := sort.Find(N, func(i int) int {
		return tx.Ops[i].Addr.CompareElementID(want)
	})
	if idx >= N {
		return ErrAttrNotFound
	}

	// check we have a match but ignore EditID
	elemID := tx.Ops[idx].Addr.ElementLSM()
	wantID := want.ElementLSM()
	if elemID != wantID {
		return ErrAttrNotFound
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
			return ErrBadTxEdit
		}
	}
	sort.Slice(tx.Ops, func(i, j int) bool {
		return tx.Ops[i].Addr.Compare(&tx.Ops[j].Addr) < 0
	})

	// TODO: validate other parts of TxMsg?

	tx.Normalized = true
	return nil
}

func (tx *TxMsg) Upsert(nodeID, attrID, itemID tag.UID, val Value) error {
	op := TxOp{
		Flags: TxOpFlags_Upsert,
	}
	op.Addr.NodeID = nodeID
	op.Addr.AttrID = attrID
	op.Addr.ItemID = itemID

	return tx.MarshalOp(&op, val)
}

func (tx *TxMsg) Delete(elemID tag.ElementID, val Value) error {
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
func (tx *TxMsg) MarshalOp(op *TxOp, val Value) error {

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
		ds, err = val.MarshalToStore(ds)
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

const (
	txBaseSize = int(Const_TxPreamble_Size) + int(unsafe.Sizeof(TxHeader{}))
	txOpSize   = int(unsafe.Sizeof(TxOp{}))
)

// Returns the ceiling byte size of this TxMsg as a serialized buffer.
func (tx *TxMsg) CeilingSize() int64 {
	sz := txBaseSize + len(tx.DataStore)
	sz += len(tx.Ops) * txOpSize
	return int64(sz)
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
		return nil, ErrMalformedTx
	}
	if preamble[3] < byte(Const_TxPreamble_Version) {
		return nil, ErrMalformedTx
	}

	tx := TxNew()
	headLen := preamble.TxHeadLen()
	dataLen := preamble.TxDataLen()

	// Use tx.DataStore as a temp store the tx header for unmarshalling, containing TxHeader and TxOps.
	{
		needSz := max(headLen, dataLen)
		if cap(tx.DataStore) < needSz {
			tx.DataStore = make([]byte, max(needSz, 2048))
		}

		buf := tx.DataStore[:headLen-int(Const_TxPreamble_Size)]
		if err := readBytes(buf); err != nil {
			return nil, err
		}
		if err := tx.UnmarshalHeader(buf); err != nil {
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

	tx.MarshalHeaderAndOps(scrap)
	if err = writeBytes(*scrap); err != nil {
		return
	}
	if err = writeBytes(tx.DataStore); err != nil {
		return
	}
	return
}

func (tx *TxMsg) MarshalToBuffer(dst *[]byte) {
	tx.MarshalHeaderAndOps(dst)
	*dst = append(*dst, tx.DataStore...)
}

func (tx *TxMsg) MarshalHeaderAndOps(dst *[]byte) {
	buf := *dst
	if cap(buf) < 300 {
		buf = make([]byte, 2048)
	}

	headerAndOps := tx.MarshalOps(buf[:Const_TxPreamble_Size])

	header := headerAndOps[:Const_TxPreamble_Size]
	header[0] = byte((Const_TxPreamble_Marker >> 16) & 0xFF)
	header[1] = byte((Const_TxPreamble_Marker >> 8) & 0xFF)
	header[2] = byte((Const_TxPreamble_Marker >> 0) & 0xFF)
	header[3] = byte(Const_TxPreamble_Version)

	binary.BigEndian.PutUint32(header[4:8], uint32(len(headerAndOps)))
	binary.BigEndian.PutUint32(header[8:12], uint32(len(tx.DataStore)))

	*dst = headerAndOps
}

func (tx *TxMsg) MarshalOps(dst []byte) []byte {

	// TxHeader
	{
		tx.OpCount = uint64(len(tx.Ops))
		infoLen := tx.TxHeader.Size()
		dst = binary.AppendUvarint(dst, uint64(infoLen))

		p := len(dst)
		dst = append(dst, make([]byte, infoLen)...)
		tx.TxHeader.MarshalToSizedBuffer(dst[p : p+infoLen])
	}

	var (
		op_prv [TxField_MaxFields]uint64
		op_cur [TxField_MaxFields]uint64
	)

	for _, op := range tx.Ops {
		dst = append(dst, byte(op.Flags))
		dst = binary.AppendUvarint(dst, 0) // skip bytes (future use)
		dst = binary.AppendUvarint(dst, op.DataLen)
		dst = binary.AppendUvarint(dst, op.DataOfs)

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

func (tx *TxMsg) UnmarshalHeader(src []byte) error {
	p := 0

	// TxHeader
	{
		infoLen, n := binary.Uvarint(src[0:])
		if n <= 0 {
			return ErrMalformedTx
		}
		p += n

		tx.TxHeader = TxHeader{}
		err := tx.TxHeader.Unmarshal(src[p : p+int(infoLen)])
		if err != nil {
			return ErrMalformedTx
		}
		p += int(infoLen)
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

		// reserved / future use
		var skip uint64
		if skip, n = binary.Uvarint(src[p:]); n <= 0 {
			return ErrMalformedTx
		}
		p += n + int(skip)

		// DataLen
		if op.DataLen, n = binary.Uvarint(src[p:]); n <= 0 {
			return ErrMalformedTx
		}
		p += n

		// DataOfs
		if op.DataOfs, n = binary.Uvarint(src[p:]); n <= 0 {
			return ErrMalformedTx
		}
		p += n

		// hasFields
		var hasFields uint64
		if hasFields, n = binary.Uvarint(src[p:]); n <= 0 {
			return ErrMalformedTx
		}
		p += n

		for j := range int(TxField_MaxFields) {
			if hasFields&(1<<j) != 0 {
				if p+8 > len(src) {
					return ErrMalformedTx
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
