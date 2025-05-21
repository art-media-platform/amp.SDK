package amp

import (
	"encoding/binary"
	"io"
	"sort"
	"sync"
	"sync/atomic"

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
	tx.refCount = 1
	return tx
}

func TxGenesis() *TxMsg {
	tx := TxNew()
	tx.SetID(tag.UID_Now())
	return tx
}

var gTxMsgPool = sync.Pool{
	New: func() interface{} {
		return &TxMsg{}
	},
}

func (tx *TxHeader) SetContextID(ID tag.UID) {
	tx.ContextID_0 = ID[0]
	tx.ContextID_1 = ID[1]
}

func (tx *TxHeader) ContextID() tag.UID {
	return tag.UID{tx.ContextID_0, tx.ContextID_1}
}

func (tx *TxHeader) SetID(ID tag.UID) {
	tx.TxID_0 = ID[0]
	tx.TxID_1 = ID[1]
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
	if ofs > end || end > uint64(len(tx.DataStore)) {
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

func (tx *TxMsg) LoadValue(findID *tag.Address, dst Value) error {
	tx.sortOps()

	if findID.ItemID.Wildcard() != 0 {
		panic("TODO") // add ItemID wildcard support
	}

	N := len(tx.Ops)
	idx, _ := sort.Find(N, func(i int) int {
		return tx.Ops[i].Addr.CompareTo(findID, false)
	})
	if idx >= N {
		return ErrAttrNotFound
	}

	// check we have a match but ignore EditID
	itemID := tx.Ops[idx].Addr.AsID()
	wantID := findID.AsID()
	if itemID != wantID {
		return ErrAttrNotFound
	}

	return tx.UnmarshalOpValue(idx, dst)
}

func (tx *TxMsg) sortOps() {
	if !tx.OpsSorted {
		tx.OpsSorted = true
		sort.Slice(tx.Ops, func(i, j int) bool {
			return tx.Ops[i].Addr.CompareTo(&tx.Ops[j].Addr, true) < 0
		})
	}
}

func (tx *TxMsg) Upsert(chanID, attrID, itemID tag.UID, val Value) error {
	op := TxOp{
		OpCode: TxOpCode_Upsert,
	}
	op.Addr.ChanID = chanID
	op.Addr.AttrID = attrID
	op.Addr.ItemID = itemID

	return tx.MarshalOp(&op, val)
}

// Marshals a TxOp and optional value to the given Tx's data store.
//
// On success:
//   - TxOp.DataOfs and TxOp.DataLen are overwritten,
//   - TxMsg.DataStore is appended with the serialization of val, and
//   - the TxOp is appended to TxMsg.Ops.
func (tx *TxMsg) MarshalOp(op *TxOp, val Value) error {
	if val == nil {
		op.DataOfs = 0
		op.DataLen = 0
	} else {
		var err error
		op.DataOfs = uint64(len(tx.DataStore))
		tx.DataStore, err = val.MarshalToStore(tx.DataStore)
		if err != nil {
			return err
		}
		op.DataLen = uint64(len(tx.DataStore)) - op.DataOfs
	}

	if op.Addr.EditID.IsNil() {
		op.Addr.EditID = tag.GenesisEditID()
	}

	tx.OpCount += 1
	tx.Ops = append(tx.Ops, *op)
	tx.OpsSorted = false
	return nil
}

func (tx *TxMsg) MarshalOpWithBuf(op *TxOp, valBuf []byte) {
	op.DataOfs = uint64(len(tx.DataStore))
	op.DataLen = uint64(len(valBuf))
	tx.DataStore = append(tx.DataStore, valBuf...)
	tx.OpCount += 1
	tx.Ops = append(tx.Ops, *op)
	tx.OpsSorted = false
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

	// Use tx.DataStore to hold 'head" for unmarshalling, containing TxMsg fields and TxOps
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
		dst = binary.AppendUvarint(dst, 0) // skip bytes (future use)
		dst = binary.AppendUvarint(dst, uint64(op.OpCode))
		dst = binary.AppendUvarint(dst, op.DataLen)
		dst = binary.AppendUvarint(dst, op.DataOfs)

		// detect repeated fields and write only what changes (with corresponding flags)
		{
			op_cur[TxField_ChanID_0] = op.Addr.ChanID[0]
			op_cur[TxField_ChanID_1] = op.Addr.ChanID[1]

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
					dst = binary.LittleEndian.AppendUint64(dst, fi)
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

		// skip (future use)
		var skip uint64
		if skip, n = binary.Uvarint(src[p:]); n <= 0 {
			return ErrMalformedTx
		}
		p += n + int(skip)

		// OpCode
		var opCode uint64
		if opCode, n = binary.Uvarint(src[p:]); n <= 0 {
			return ErrMalformedTx
		}
		p += n
		op.OpCode = TxOpCode(opCode)

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
				op_cur[j] = binary.LittleEndian.Uint64(src[p:])
				p += 8
			}
		}

		op.Addr.ChanID[0] = op_cur[TxField_ChanID_0]
		op.Addr.ChanID[1] = op_cur[TxField_ChanID_1]

		op.Addr.AttrID[0] = op_cur[TxField_AttrID_0]
		op.Addr.AttrID[1] = op_cur[TxField_AttrID_1]

		op.Addr.ItemID[0] = op_cur[TxField_ItemID_0]
		op.Addr.ItemID[1] = op_cur[TxField_ItemID_1]

		op.Addr.EditID[0] = op_cur[TxField_EditID_0]
		op.Addr.EditID[1] = op_cur[TxField_EditID_1]

		tx.Ops = append(tx.Ops, op)
	}
	tx.OpsSorted = false

	return nil
}
