package amp

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

func TestTxSerialize(t *testing.T) {
	// Test serialization of a simple TxMsg

	tx := TxGenesis()
	tx.Status = PinStatus_Syncing
	tx.SetContextID(tag.UID{0x1234567890abcdef, 0xabcdef1234567890})

	{
		op := TxOp{
			OpCode: TxOpCode_Upsert,
			Addr: tag.Address{
				ChanID: tag.U3D{99923456789, 987621, 11123456789},
				AttrID: tag.UID{111312232, 22232334444},
				ItemID: tag.U3D{7383, 76549, 3773},
				EditID: tag.UID{4435435, 83849854543},
			},
		}

		tx.MarshalOp(&op, &Login{
			User: &Tag{
				Text: "astar",
			},
			HostAddress: "batwing ave",
		})
		tx.DataStore = append(tx.DataStore, []byte("bytes not used but stored -- not normal!")...)

		op.Addr.ChanID[0] += 321
		op.Addr.ChanID[1] -= 212
		op.Addr.ChanID[2] += 37733773
		op.Addr.AttrID[1] -= 50454123
		op.Addr.ItemID[2] += 323
		data := []byte("hello-world")
		for i := 0; i < 7; i++ {
			data = append(data, data...)
		}
		tx.MarshalOp(&op, &Login{
			User: &Tag{
				Text: "anonymous",
			},
			HostAddress: "http://localhost:8080",
		})

		for i := 0; i < 5500; i++ {
			op.Addr.ItemID[0] = uint64(i)
			if i%5 == 0 {
				op.Addr.EditID[1] += 37
			}
			tx.MarshalOp(&op, &LoginResponse{
				HashResponse: append(data, fmt.Sprintf("-%d", i)...),
			})
		}

		op.Addr.ItemID[0] = 111111
		op.Addr.EditID[1] = 55445544
		op.OpCode = TxOpCode_Delete
		tx.MarshalOpWithBuf(&op, nil)
	}

	var txBuf []byte
	tx.MarshalToBuffer(&txBuf)

	r := bufReader{
		buf: txBuf,
	}
	t2, err := ReadTxMsg(&r)
	if err != nil {
		t.Errorf("ReadTxMsg failed: %v", err)
	}
	h1, _ := tx.TxHeader.Marshal()
	h2, _ := t2.TxHeader.Marshal()
	if !bytes.Equal(h1, h2) {
		t.Errorf("ReadTxMsg failed: TxHeader mismatch")
	}
	if len(tx.Ops) != len(t2.Ops) {
		t.Errorf("ReadTxMsg failed: TxHeader mismatch")
	}
	if !bytes.Equal(tx.DataStore, t2.DataStore) {
		t.Errorf("ReadTxMsg failed: DataStore mismatch")
	}
	for i, op1 := range tx.Ops {
		op2 := t2.Ops[i]

		if op1.OpCode != op2.OpCode || op1 != op2 || op1.DataOfs != op2.DataOfs || op1.DataLen != op2.DataLen {
			t.Errorf("ReadTxMsg failed: TxOp mismatch")
		}
	}
}

type bufReader struct {
	buf []byte
	pos int
}

func (r *bufReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.buf) {
		return 0, io.EOF
	}
	n = copy(p, r.buf[r.pos:])
	r.pos += n
	return n, nil
}
