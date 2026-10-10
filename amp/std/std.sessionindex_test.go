package std

import (
	"context"
	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// pushRecorder is a session whose pushes are recorded; every other
// amp.Session method is unimplemented (nil embed) and unreached.
type pushRecorder struct {
	amp.Session
	pushed []*amp.TxMsg
}

func (rec *pushRecorder) NewTx(scope ...amp.TxScope) *amp.TxMsg {
	return amp.TxNew()
}

func (rec *pushRecorder) PushTx(tx *amp.TxMsg, guard context.Context) error {
	rec.pushed = append(rec.pushed, tx)
	return nil
}

// pushedItems reports each recorded push's one op as itemID → deleted.
func (rec *pushRecorder) pushedItems(t *testing.T, attrID tag.UID) map[tag.UID]bool {
	t.Helper()
	items := make(map[tag.UID]bool)
	for _, tx := range rec.pushed {
		if len(tx.Ops) != 1 {
			t.Fatalf("push carries %d ops, want 1", len(tx.Ops))
		}
		op := tx.Ops[0]
		if op.Flags&amp.TxOpFlags_MetaOp == 0 {
			t.Fatal("index push is not a meta op: a client would split it off as a planet merge")
		}
		if op.Addr.NodeID != amp.HeadNodeID || op.Addr.AttrID != attrID {
			t.Fatalf("push addressed %v, want (HeadNodeID, attr)", op.Addr.ElementID)
		}
		if tx.ContextID() != SessionContextID || tx.Status != amp.PinStatus_Synced {
			t.Fatalf("push context %v status %v, want the session context, Synced", tx.ContextID(), tx.Status)
		}
		items[op.Addr.ItemID] = op.Flags&amp.TxOpFlags_Delete != 0
	}
	return items
}

func TestSessionIndex_HoldsUntilOpen(t *testing.T) {
	rec := &pushRecorder{}
	idx := SessionIndex{Attr: Attr.PlanetBinding.ID}
	kept, dropped, late := tag.NewID(), tag.NewID(), tag.NewID()

	idx.Put(kept, &amp.Tag{Text: "kept"})
	idx.Put(dropped, &amp.Tag{Text: "dropped"})
	idx.Forget(dropped)
	if len(rec.pushed) != 0 {
		t.Fatalf("%d push(es) before Open — a pre-grant push completes a client's login early", len(rec.pushed))
	}

	idx.Open(rec)
	items := rec.pushedItems(t, Attr.PlanetBinding.ID)
	if len(items) != 1 || items[kept] {
		t.Fatalf("Open pushed %v, want the one held item %v as an upsert", items, kept)
	}

	idx.Open(rec)
	if len(rec.pushed) != 1 {
		t.Fatalf("a second Open pushed again (%d pushes)", len(rec.pushed))
	}

	idx.Put(late, &amp.Tag{Text: "late"})
	idx.Forget(kept)
	items = rec.pushedItems(t, Attr.PlanetBinding.ID)
	if deleted, pushed := items[late]; !pushed || deleted {
		t.Fatalf("Put after Open did not push an upsert: %v", items)
	}
	if deleted := items[kept]; !deleted {
		t.Fatalf("Forget after Open did not push a delete: %v", items)
	}
}
