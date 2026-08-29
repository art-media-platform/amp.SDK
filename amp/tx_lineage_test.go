package amp_test

// tx_lineage_test.go — the explicit lineage authoring rail (AOM
// SD-edit-resolution.md §6.1): UpsertFrom / FoldBinding.UpsertItemFrom frame
// the caller's loaded base as the ParentEdit inline UID
// (ValueHeaderFlags_UID_C) in the op value header; the EditID stays the TxID
// (parent seeding is a header stamp, never an EditID seed), and a nil base
// degrades to the plain wildcard Upsert.

import (
	"bytes"
	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/amp/std"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"google.golang.org/protobuf/proto"
)

func TestUpsertFrom_FramesParentEdit(t *testing.T) {
	node := tag.NowID()
	item := tag.NowID()
	txID := tag.NowID()
	from := tag.NowID()
	base := tag.NowID()

	tx := amp.TxNew()
	tx.SetTxID(txID)
	tx.SetFromID(from)
	if err := tx.UpsertFrom(node, std.Attr.PlanetBinding.ID, item, base, &amp.Tag{Text: "parented"}); err != nil {
		t.Fatal(err)
	}

	op := tx.Ops[0]
	value := tx.DataStore[op.DataOfs : op.DataOfs+op.DataLen]
	wantFlags := byte(amp.ValueHeaderFlags_FromID | amp.ValueHeaderFlags_UID_C)
	if value[0] != wantFlags {
		t.Fatalf("header flags = 0x%02x, want 0x%02x (FromID|UID_C)", value[0], wantFlags)
	}
	// Inline UIDs ride in ascending flag-bit order: FromID (0x01) then ParentEdit (0x04).
	fromBytes := from.AppendTo(nil)
	baseBytes := base.AppendTo(nil)
	if !bytes.Equal(value[1:1+tag.UID_Size], fromBytes) {
		t.Fatal("FromID inline UID not first after the flags byte")
	}
	if !bytes.Equal(value[1+tag.UID_Size:1+2*tag.UID_Size], baseBytes) {
		t.Fatal("ParentEdit inline UID not second (ascending flag-bit order)")
	}
	if op.Addr.EditID != txID {
		t.Fatalf("EditID = %v, want TxID %v — parent seeding must never move the EditID", op.Addr.EditID, txID)
	}

	// The header skip serves the value bytes unchanged.
	decoded := &amp.Tag{}
	if err := tx.UnmarshalOpValue(0, decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Text != "parented" {
		t.Fatalf("decoded value %q, want %q", decoded.Text, "parented")
	}
}

func TestUpsertFrom_NilBaseIsWildcard(t *testing.T) {
	node := tag.NowID()
	item := tag.NowID()

	parented := amp.TxNew()
	parented.SetTxID(tag.NowID())
	if err := parented.UpsertFrom(node, std.Attr.PlanetBinding.ID, item, tag.UID{}, &amp.Tag{Text: "x"}); err != nil {
		t.Fatal(err)
	}
	plain := amp.TxNew()
	plain.SetTxID(parented.TxID())
	if err := plain.Upsert(node, std.Attr.PlanetBinding.ID, item, &amp.Tag{Text: "x"}); err != nil {
		t.Fatal(err)
	}

	nilBaseValue := parented.DataStore[parented.Ops[0].DataOfs : parented.Ops[0].DataOfs+parented.Ops[0].DataLen]
	plainValue := plain.DataStore[plain.Ops[0].DataOfs : plain.Ops[0].DataOfs+plain.Ops[0].DataLen]
	if !bytes.Equal(nilBaseValue, plainValue) {
		t.Fatal("UpsertFrom with a nil base must frame byte-identically to Upsert (deliberate wildcard)")
	}
	if nilBaseValue[0]&byte(amp.ValueHeaderFlags_UID_C) != 0 {
		t.Fatal("nil base must not set the ParentEdit header flag")
	}
}

func TestFoldBinding_UpsertItemFrom(t *testing.T) {
	node := tag.NowID()
	item := tag.NowID()
	base := tag.NowID()
	txID := tag.NowID()

	binding := bindTag(node)
	tx := amp.TxNew()
	tx.SetTxID(txID)
	if err := binding.UpsertItemFrom(tx, item, base, &amp.Tag{Text: "via-binding"}); err != nil {
		t.Fatal(err)
	}

	op := tx.Ops[0]
	if op.Addr.NodeID != node || op.Addr.ItemID != item || op.Addr.EditID != txID {
		t.Fatalf("op address (%v %v %v) does not carry the binding context", op.Addr.NodeID, op.Addr.ItemID, op.Addr.EditID)
	}
	value := tx.DataStore[op.DataOfs : op.DataOfs+op.DataLen]
	if value[0]&byte(amp.ValueHeaderFlags_UID_C) == 0 {
		t.Fatal("UpsertItemFrom must frame the ParentEdit header flag")
	}
	if !bytes.Equal(value[1+tag.UID_Size:1+2*tag.UID_Size], base.AppendTo(nil)) {
		t.Fatal("UpsertItemFrom must frame base as the ParentEdit inline UID")
	}
	decoded := &amp.Tag{}
	if err := tx.UnmarshalOpValue(0, decoded); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(decoded, &amp.Tag{Text: "via-binding"}) {
		t.Fatalf("decoded value %v diverges from the authored one", decoded)
	}
}
