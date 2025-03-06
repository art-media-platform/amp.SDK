package amp

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

func TestTxSerialize(t *testing.T) {
	// Test serialization of a simple TxMsg

	tx := NewTxMsg(true)
	tx.Status = OpStatus_Syncing
	tx.SetContextID(tag.ID{
		888854513,
		7777435,
		77743773,
	})

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
				Text: "cmdr5",
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
				Text: "anonymous",
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
	if tx2.TxHeader != tx.TxHeader {
		t.Errorf("ReadTxMsg failed: TxHeader mismatch")
	}
	if len(tx2.Ops) != len(tx.Ops) {
		t.Errorf("ReadTxMsg failed: TxHeader mismatch")
	}
	if !bytes.Equal(tx.DataStore, tx2.DataStore) {
		t.Errorf("ReadTxMsg failed: DataStore mismatch")
	}
	for i, op1 := range tx.Ops {
		op2 := tx2.Ops[i]

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

func TestRegistry(t *testing.T) {
	reg := NewRegistry()
	someAttr := tag.Expr{}.With("hello sailor")
	spec := AttrDef{
		Expr:      someAttr.With("av.Hello.World.Tag"),
		Prototype: &Tag{},
	}

	{
		err := reg.RegisterAttr(spec)
		if err != nil {
			t.Fatalf("RegisterAttr failed: %v", err)
		}
		elem, err := reg.MakeValue(spec.ID)
		if err != nil {
			t.Fatalf("MakeValue failed: %v", err)
		}
		if spec.Canonic != someAttr.Canonic+".av.hello.world.tag" {
			t.Fatal("RegisterAttr failed")
		}
		if reflect.TypeOf(elem) != reflect.TypeOf(&Tag{}) {
			t.Fatalf("MakeValue returned wrong type: %v", reflect.TypeOf(elem))
		}
	}

	if spec.ID != (tag.Expr{}.With("hello.sailor.World.Tag.Hello.av")).ID {
		t.Fatalf("tag.With failed")
	}
	alias := someAttr.With("av").With("World.Hello.Tag")
	if spec.ID != alias.ID {
		t.Fatalf("tag.With failed")
	}
	if str := spec.ID.Base32Suffix(); str != "2Y227W6E" {
		t.Fatalf("unexpected spec.ID: %v", str)
	}
	if (tag.ID{}).Base32() != "0" {
		t.Fatalf("tag.Expr{}.Base32() failed")
	}
	if str := spec.ID.Base32(); str != "1RRFCSNXUZ9YW2T5KF8YJYV4C2Y227W6E" {
		t.Errorf("tag.ID.Base32() failed: %v", str)
	}
	if str := spec.ID.Base16(); str != "1bddcbc53bafa7dc164b2723d1f6c8b178423f0cd" {
		t.Errorf("tag.ID.Base16() failed: %v", str)
	}

}
