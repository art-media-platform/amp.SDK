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
// It is leads with a fixed-size header (TxHeader_Size) followed by a variable-size body.
type TxDataStore []byte

// TxHeader is the fixed-size header that leads every TxMsg.
// See comments for Const_TxHeader_Size.
type TxHeader [Const_TxHeader_Size]byte

func (header TxHeader) TxBodyLen() int {
	return int(binary.LittleEndian.Uint32(header[4:8]))
}

func (header TxHeader) TxDataLen() int {
	return int(binary.LittleEndian.Uint32(header[8:12]))
}

func NewTxMsg(genesis bool) *TxMsg {
	tx := gTxMsgPool.Get().(*TxMsg)
	tx.refCount = 1
	if genesis {
		tid := tag.Now()
		tx.GenesisID_0 = int64(tid[0])
		tx.GenesisID_1 = tid[1]
		tx.GenesisID_2 = tid[2]
	}

	return tx
}

var gTxMsgPool = sync.Pool{
	New: func() interface{} {
		return &TxMsg{}
	},
}

func (tx *TxEnvelope) SetContextID(ID tag.ID) {
	tx.ContextID_0 = int64(ID[0])
	tx.ContextID_1 = ID[1]
	tx.ContextID_2 = ID[2]
}

func (tx *TxEnvelope) ContextID() tag.ID {
	return tag.ID{uint64(tx.ContextID_0), tx.ContextID_1, tx.ContextID_2}
}

func (tx *TxEnvelope) SetGenesisID(ID tag.ID) {
	tx.GenesisID_0 = int64(ID[0])
	tx.GenesisID_1 = ID[1]
	tx.GenesisID_2 = ID[2]
}

func (tx *TxEnvelope) GenesisID() tag.ID {
	return tag.ID{uint64(tx.GenesisID_0), tx.GenesisID_1, tx.GenesisID_2}
}

// // If this were Java, I'd call this a TxMsgBuilder
// func (tx *TxMsg) AddRef() tag.ID {
// func (tx *TxMsg) Witnessed() tag.ID {
// 	atomic.AddInt32(&tx.refCount, 1)
// }

func (tx *TxMsg) AddRef() {
	atomic.AddInt32(&tx.refCount, 1)
}

func (tx *TxMsg) ReleaseRef() {
	if atomic.AddInt32(&tx.refCount, -1) > 0 {
		return
	}

	*tx = TxMsg{
		Ops:       tx.Ops[:0],
		DataStore: tx.DataStore[:0],
	}
	gTxMsgPool.Put(tx)
}

func MarshalAttr(cellID, attrID tag.ID, attrVal tag.Value) (*TxMsg, error) {
	tx := NewTxMsg(true)
	if attrID.IsNil() && attrVal != nil {
		attrID = attrVal.TagSpec().ID
		if attrID.IsNil() {
			return nil, ErrCode_InternalErr.Error("MarshalAttr: missing builtin tag.Spec")
		}
	}
	op := TxOp{}
	op.CellID = cellID
	op.AttrID = attrID
	op.EditID = tag.Genesis(tx.GenesisID())
	op.OpCode = TxOpCode_UpsertElement

	if err := tx.MarshalOp(&op, attrVal); err != nil {
		return nil, err
	}
	return tx, nil
}

func (tx *TxMsg) UnmarshalOpValue(idx int, out tag.Value) error {
	if idx < 0 || idx >= len(tx.Ops) {
		return ErrCode_MalformedTx.Error("UnmarshalOpValue: index out of range")
	}
	op := tx.Ops[idx]
	ofs := op.DataOfs
	span := tx.DataStore[ofs : ofs+op.DataLen]
	return out.Unmarshal(span)
}

func (tx *TxMsg) LoadItem(attrID, itemID tag.ID, dst tag.Value) error {
	for i, op := range tx.Ops {
		if op.AttrID == attrID && op.ItemID == itemID {
			return tx.UnmarshalOpValue(i, dst)
		}
	}
	return ErrAttrNotFound
}

func (tx *TxMsg) Load(cellID, attrID, itemID tag.ID, dst tag.Value) error {
	tx.sortOps()

	find := &TxOpID{
		CellID: cellID,
		AttrID: attrID,
		ItemID: itemID,
	}
	idx, found := sort.Find(len(tx.Ops), func(i int) int {
		return tx.Ops[i].CompareTo(find)
	})
	if !found {
		return ErrPropertyNotFound
	}

	return tx.UnmarshalOpValue(idx, dst)
}

var (
	ErrAttrNotFound     = ErrCode_BadRequest.Error("attribute not found")
	ErrPropertyNotFound = ErrCode_BadRequest.Error("property not found")
)

func (tx *TxMsg) sortOps() {
	if !tx.OpsSorted {
		tx.OpsSorted = true
		sort.Slice(tx.Ops, func(i, j int) bool {
			return tx.Ops[i].TxOpID.CompareTo(&tx.Ops[j].TxOpID) < 0
		})
	}
}

// If reqID == 0, then this sends an attr to the client's session controller (vs a specific request)
func SendMetaAttr(sess Session, context tag.ID, status OpStatus, attrID tag.ID, val tag.Value) error {
	tx, err := MarshalAttr(MetaNodeID, attrID, val)
	if err != nil {
		return err
	}
	tx.SetContextID(context)
	tx.Status = status
	return sess.SendTx(tx)
}

// If nil, nil is returned, then this Tx is a valid TxMsg to be merged into the target Pin.
func (tx *TxMsg) CheckMetaAttr(reg Registry) (tag.Value, error) {
	genesisID := tx.GenesisID()
	if genesisID.IsNil() {
		return nil, ErrCode_MalformedTx.Error("missing tx.GenesisID")
	}
	if len(tx.Ops) != 1 || tx.Ops[0].CellID != MetaNodeID {
		return nil, nil
	}
	val, err := reg.MakeValue(tx.Ops[0].AttrID)
	if err != nil {
		return nil, err
	}
	if err = tx.UnmarshalOpValue(0, val); err != nil {
		return nil, err
	}
	return val, nil
}

func (tx *TxMsg) Upsert(cellID, attrID, itemID tag.ID, val tag.Value) error {
	op := TxOp{}
	op.OpCode = TxOpCode_UpsertElement
	op.CellID = cellID
	op.AttrID = attrID
	op.ItemID = itemID
	op.EditID = tag.Genesis(tx.GenesisID())

	return tx.MarshalOp(&op, val)
}

// Marshals a TxOp and optional value to the given Tx's to and data store.
//
// On success:
//   - TxOp.DataOfs and TxOp.DataLen are overwritten,
//   - TxMsg.DataStore is appended with the serialization of val, and
//   - the TxOp is appended to TxMsg.Ops.
func (tx *TxMsg) MarshalOp(op *TxOp, val tag.Value) error {
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

	tx.OpCount += 1
	tx.Ops = append(tx.Ops, *op)
	return nil
}

func (tx *TxMsg) MarshalOpWithBuf(op *TxOp, valBuf []byte) {
	op.DataOfs = uint64(len(tx.DataStore))
	op.DataLen = uint64(len(valBuf))
	tx.DataStore = append(tx.DataStore, valBuf...)
	tx.OpCount += 1
	tx.Ops = append(tx.Ops, *op)
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

	var header TxHeader
	if err := readBytes(header[:]); err != nil {
		return nil, err
	}

	marker := uint32(header[0])<<16 | uint32(header[1])<<8 | uint32(header[2])
	if marker != uint32(Const_TxHeader_Marker) {
		return nil, ErrMalformedTx
	}
	if header[3] < byte(Const_TxHeader_Version) {
		return nil, ErrMalformedTx
	}

	tx := NewTxMsg(false)
	bodyLen := header.TxBodyLen()
	dataLen := header.TxDataLen()

	// Use tx.DataStore to hold the body for unmarshalling.
	// The tx body contains TxMsg fields and TxOps
	{
		needSz := max(bodyLen, dataLen)
		if cap(tx.DataStore) < needSz {
			tx.DataStore = make([]byte, max(needSz, 2048))
		}

		buf := tx.DataStore[:bodyLen-int(Const_TxHeader_Size)]
		if err := readBytes(buf); err != nil {
			return nil, err
		}
		if err := tx.UnmarshalBody(buf); err != nil {
			return nil, err
		}
	}

	// Read tx data store -- used for on-demand tag.Value unmarshalling
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
	buf := (*dst)[:0]
	if cap(buf) < 300 {
		buf = make([]byte, 2048)
	}

	headerAndOps := tx.MarshalOps(buf[:Const_TxHeader_Size])

	header := headerAndOps[:Const_TxHeader_Size]
	header[0] = byte((Const_TxHeader_Marker >> 16) & 0xFF)
	header[1] = byte((Const_TxHeader_Marker >> 8) & 0xFF)
	header[2] = byte((Const_TxHeader_Marker >> 0) & 0xFF)
	header[3] = byte(Const_TxHeader_Version)

	binary.LittleEndian.PutUint32(header[4:8], uint32(len(headerAndOps)))
	binary.LittleEndian.PutUint32(header[8:12], uint32(len(tx.DataStore)))

	*dst = headerAndOps
}

func (tx *TxMsg) MarshalOps(dst []byte) []byte {

	// TxEnvelope
	{
		tx.OpCount = uint64(len(tx.Ops))
		infoLen := tx.TxEnvelope.Size()
		dst = binary.AppendUvarint(dst, uint64(infoLen))

		p := len(dst)
		dst = append(dst, make([]byte, infoLen)...)
		tx.TxEnvelope.MarshalToSizedBuffer(dst[p : p+infoLen])
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

		// detect body repeated fields and write only what changes (with corresponding flags)
		{
			op_cur[TxField_CellID_0] = op.CellID[0]
			op_cur[TxField_CellID_1] = op.CellID[1]
			op_cur[TxField_CellID_2] = op.CellID[2]

			op_cur[TxField_AttrID_0] = op.AttrID[0]
			op_cur[TxField_AttrID_1] = op.AttrID[1]
			op_cur[TxField_AttrID_2] = op.AttrID[2]

			op_cur[TxField_ItemID_0] = op.ItemID[0]
			op_cur[TxField_ItemID_1] = op.ItemID[1]
			op_cur[TxField_ItemID_2] = op.ItemID[2]

			op_cur[TxField_EditID_0] = op.EditID[0]
			op_cur[TxField_EditID_1] = op.EditID[1]
			op_cur[TxField_EditID_2] = op.EditID[2]

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

			op_prv = op_cur
		}
	}

	return dst
}

func (tx *TxMsg) UnmarshalBody(src []byte) error {
	p := 0

	// TxEnvelope
	{
		infoLen, n := binary.Uvarint(src[0:])
		if n <= 0 {
			return ErrMalformedTx
		}
		p += n

		tx.TxEnvelope = TxEnvelope{}
		err := tx.TxEnvelope.Unmarshal(src[p : p+int(infoLen)])
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

		for i := 0; i < int(TxField_MaxFields); i++ {
			if hasFields&(1<<i) != 0 {
				if p+8 > len(src) {
					return ErrMalformedTx
				}
				op_cur[i] = binary.LittleEndian.Uint64(src[p:])
				p += 8
			}
		}

		op.CellID[0] = op_cur[TxField_CellID_0]
		op.CellID[1] = op_cur[TxField_CellID_1]
		op.CellID[2] = op_cur[TxField_CellID_2]

		op.AttrID[0] = op_cur[TxField_AttrID_0]
		op.AttrID[1] = op_cur[TxField_AttrID_1]
		op.AttrID[2] = op_cur[TxField_AttrID_2]

		op.ItemID[0] = op_cur[TxField_ItemID_0]
		op.ItemID[1] = op_cur[TxField_ItemID_1]
		op.ItemID[2] = op_cur[TxField_ItemID_2]

		op.EditID[0] = op_cur[TxField_EditID_0]
		op.EditID[1] = op_cur[TxField_EditID_1]
		op.EditID[2] = op_cur[TxField_EditID_2]

		tx.Ops = append(tx.Ops, op)
	}

	return nil
}

func (op *TxOpID) CompareTo(oth *TxOpID) int {
	if diff := op.CellID.CompareTo(oth.CellID); diff != 0 {
		return int(diff)
	}
	if diff := op.AttrID.CompareTo(oth.AttrID); diff != 0 {
		return int(diff)
	}
	if diff := op.ItemID.CompareTo(oth.ItemID); diff != 0 {
		return int(diff)
	}
	if diff := op.EditID.CompareTo(oth.EditID); diff != 0 {
		return int(diff)
	}
	return 0
}
