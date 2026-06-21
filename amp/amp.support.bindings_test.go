package amp_test

import (
	"testing"
	"time"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/amp/std"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// bindTag returns a wildcard *amp.Tag binding bound to node.
func bindTag(node tag.UID) *amp.AttrBinding[*amp.Tag] {
	binding := amp.NewAttrBinding[*amp.Tag](std.Attr.PlanetBinding)
	binding.Bind(node)
	return binding
}

// feedUpsert drives one Upsert(node, PlanetBinding, item)=text op through the
// binding under the witnessed txID.  SetTxID precedes Upsert so the op's EditID
// derives from txID (DeriveID(nil) == txID), giving txID-ordered EditIDs.
func feedUpsert(binding *amp.AttrBinding[*amp.Tag], node, item, txID tag.UID, text string) {
	tx := amp.TxNew()
	tx.SetTxID(txID)
	_ = tx.Upsert(node, std.Attr.PlanetBinding.ID, item, &amp.Tag{Text: text})
	binding.OnNodeUpdate(amp.NodeUpdate{NodeID: node, Revision: txID, Tx: tx})
}

func tagText(value *amp.Tag) string {
	if value == nil {
		return ""
	}
	return value.Text
}

// TestAttrBinding_OnAdmitVetoBeforeEditBump pins that OnAdmit runs BEFORE the CRDT
// edit-ordering bump: a vetoed op leaves the item's high-water EditID untouched, so a
// subsequent admitted op with a LOWER EditID still applies (no cell poisoning).
func TestAttrBinding_OnAdmitVetoBeforeEditBump(t *testing.T) {
	node := tag.NowID()
	item := tag.NowID()
	lowTxID := tag.UID_FromTime(time.Now())
	highTxID := tag.UID_FromTime(time.Now().Add(time.Hour)) // larger derived EditID

	binding := bindTag(node)
	binding.OnAdmit = func(addr tag.Address, tx *amp.TxMsg) bool {
		return tx.TxID() != highTxID // veto the high-EditID write
	}

	feedUpsert(binding, node, item, highTxID, "vetoed")
	if _, ok := binding.GetItem(item); ok {
		t.Fatal("a vetoed op must not be cached")
	}

	feedUpsert(binding, node, item, lowTxID, "admitted")
	got, ok := binding.GetItem(item)
	if !ok || tagText(got) != "admitted" {
		t.Fatalf("a lower-EditID op after a vetoed higher one must apply (cell poisoned): got=%q ok=%v", tagText(got), ok)
	}
}

// TestAttrBinding_ItemTxExposesWitnessedTxID pins that OnItem can read the carrying
// tx's TxID via AttrItem.Tx — the seam app.nameservice uses to rebind a record's
// RegisteredAt to the controller-witnessed TxID.
func TestAttrBinding_ItemTxExposesWitnessedTxID(t *testing.T) {
	node := tag.NowID()
	item := tag.NowID()
	txID := tag.NowID()

	binding := bindTag(node)
	seen := tag.UID{}
	binding.OnItem = func(updated amp.AttrItem[*amp.Tag]) {
		if updated.Tx != nil {
			seen = updated.Tx.TxID()
		}
	}

	feedUpsert(binding, node, item, txID, "x")
	if seen != txID {
		t.Errorf("AttrItem.Tx.TxID() = %v, want witnessed %v", seen, txID)
	}
}

// TestAttrBinding_DeleteFiresOnItemDeleted pins the delete branch the resolver's
// reverse-index maintenance relies on: a delete op is consulted by OnAdmit, fires
// OnItem with Deleted=true, and removes the live value.
func TestAttrBinding_DeleteFiresOnItemDeleted(t *testing.T) {
	node := tag.NowID()
	item := tag.NowID()

	binding := bindTag(node)
	admits := 0
	binding.OnAdmit = func(addr tag.Address, tx *amp.TxMsg) bool { admits++; return true }
	deletes := 0
	binding.OnItem = func(updated amp.AttrItem[*amp.Tag]) {
		if updated.Deleted {
			deletes++
		}
	}

	feedUpsert(binding, node, item, tag.UID_FromTime(time.Now()), "live")
	if !binding.HasItem(item) {
		t.Fatal("upsert should be cached")
	}

	tx := amp.TxNew()
	tx.SetTxID(tag.UID_FromTime(time.Now().Add(time.Minute))) // later → delete supersedes
	if !binding.DeleteItem(tx, item) {
		t.Fatal("DeleteItem should append a delete op for a known item")
	}
	binding.OnNodeUpdate(amp.NodeUpdate{NodeID: node, Tx: tx})

	if binding.HasItem(item) {
		t.Error("a deleted item must be removed from the live cache")
	}
	if deletes != 1 {
		t.Errorf("OnItem(Deleted) fired %d times, want 1", deletes)
	}
	if admits < 2 {
		t.Errorf("OnAdmit consulted %d times, want >= 2 (upsert + delete)", admits)
	}
}
