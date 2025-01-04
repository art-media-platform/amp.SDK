package amp

import (
	"bytes"
	fmt "fmt"
	io "io"
	"reflect"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

func TestTxSerialize(t *testing.T) {
	// Test serialization of a simple TxMsg

	tx := NewTxMsg(true)
	tx.Status = OpStatus_Syncing
	tx.ContextID_0 = 888854513
	tx.ContextID_1 = 7777435
	tx.ContextID_2 = 77743773
	{
		op := TxOp{
			OpCode: TxOpCode_UpsertElement,
			TxOpID: TxOpID{
				tag.ID{3, 37, 73},
				tag.ID{111312232, 22232334444, 4321},
				tag.ID{7383, 76549, 3773},
				tag.ID{7337, 3773, 7337},
			},
		}

		tx.MarshalOp(&op, &Login{
			UserID: &Tag{
				UID: "cmdr5",
			},
			HostAddress: "batwing ave",
		})
		tx.DataStore = append(tx.DataStore, []byte("bytes not used but stored -- not normal!")...)

		op.CellID[0] += 37733773
		op.AttrID[1] -= 50454123
		op.ItemID[2] += 323
		data := []byte("hello-world")
		for i := 0; i < 7; i++ {
			data = append(data, data...)
		}
		tx.MarshalOp(&op, &Login{
			UserID: &Tag{
				UID: "anonymous",
			},
			HostAddress: "http://localhost:8080",
		})

		for i := 0; i < 5500; i++ {
			op.ItemID[0] = uint64(i)
			if i%5 == 0 {
				op.EditID[1] += 37
			}
			tx.MarshalOp(&op, &LoginResponse{
				HashResponse: append(data, fmt.Sprintf("-%d", i)...),
			})
		}

		op.ItemID[0] = 111111
		op.EditID[1] = 55445544
		op.OpCode = TxOpCode_DeleteElement
		tx.MarshalOpWithBuf(&op, nil)
	}

	var txBuf []byte
	tx.MarshalToBuffer(&txBuf)

	r := bufReader{
		buf: txBuf,
	}
	tx2, err := ReadTxMsg(&r)
	if err != nil {
		t.Errorf("ReadTxMsg failed: %v", err)
	}
	if tx2.TxEnvelope != tx.TxEnvelope {
		t.Errorf("ReadTxMsg failed: TxEnvelope mismatch")
	}
	if len(tx2.Ops) != len(tx.Ops) {
		t.Errorf("ReadTxMsg failed: TxEnvelope mismatch")
	}
	if !bytes.Equal(tx.DataStore, tx2.DataStore) {
		t.Errorf("ReadTxMsg failed: DataStore mismatch")
	}
	for i, op1 := range tx.Ops {
		op2 := tx2.Ops[i]

		if op1.OpCode != op2.OpCode || op1 != op2 || op1.DataOfs != op2.DataOfs || op1.DataLen != op2.DataLen {
			t.Errorf("ReadTxMsg failed: Op mismatch")
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

func TestRegistry(t *testing.T) {
	reg := NewRegistry()
	spec := reg.RegisterPrototype(SystemAttr.With("av.Hello.World"), &Tag{}, "")
	if spec.Canonic != SystemAttr.Canonic+".av.Hello.World.Tag" {
		t.Fatal("RegisterPrototype failed")
	}
	if spec.ID != (tag.Expr{}.With("amp.attr.World.Tag.Hello.av")).ID {
		t.Fatalf("tag.FormSpec failed")
	}
	if spec.ID != SystemAttr.With("av").With("World.Hello.Tag").ID {
		t.Fatalf("tag.FormSpec failed")
	}
	if spec.ID.Base32Suffix() != "yg70m6cn" {
		t.Fatalf("unexpected spec.ID: %v", spec.ID)
	}
	if (tag.ID{}).Base32() != "0" {
		t.Fatalf("tag.Expr{}.Base32() failed")
	}
	if spec.ID.Base32() != "3t1sm7v1ycu8rzmysqc2d93wsyg70m6cn" {
		t.Errorf("tag.ID.Base32() failed: %v", spec.ID.Base32())
	}
	if spec.ID.Base16() != "3c87133ec3e5e917fcfd8b2c4c48f98f3ce099974" {
		t.Errorf("tag.ID.Base16() failed: %v", spec.ID.Base16())
	}
	elem, err := reg.MakeValue(spec.ID)
	if err != nil {
		t.Fatalf("MakeValue failed: %v", err)
	}
	if reflect.TypeOf(elem) != reflect.TypeOf(&Tag{}) {
		t.Fatalf("MakeValue returned wrong type: %v", reflect.TypeOf(elem))
	}
}
