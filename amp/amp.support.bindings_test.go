package amp_test

import (
	"testing"
	"time"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/amp/std"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// bindTag returns a wildcard *amp.Tag binding bound to node.
func bindTag(node tag.UID) *amp.FoldBinding[*amp.Tag] {
	binding := amp.NewFoldBinding[*amp.Tag](std.Attr.PlanetBinding)
	binding.Bind(node)
	return binding
}

// feedUpsert drives one Upsert(node, PlanetBinding, item)=text op through the
// binding under the witnessed txID.  SetTxID precedes Upsert so the op's EditID
// derives from txID (DeriveID(nil) == txID), giving txID-ordered EditIDs.
func feedUpsert(binding *amp.FoldBinding[*amp.Tag], node, item, txID tag.UID, text string) {
	tx := amp.TxNew()
	tx.SetTxID(txID)
	_ = tx.Upsert(node, std.Attr.PlanetBinding.ID, item, &amp.Tag{Text: text})
	binding.OnNodeUpdate(amp.NodeUpdate{NodeID: node, Tx: tx})
}

func tagText(value *amp.Tag) string {
	if value == nil {
		return ""
	}
	return value.Text
}

// TestFoldBinding_OnAdmitVetoBeforeEditBump pins that OnAdmit runs BEFORE the CRDT
// edit-ordering bump: a vetoed op leaves the item's high-water EditID untouched, so a
// subsequent admitted op with a LOWER EditID still applies (no cell poisoning).
func TestFoldBinding_OnAdmitVetoBeforeEditBump(t *testing.T) {
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

// TestFoldBinding_ItemTxExposesWitnessedTxID pins that OnItem can read the carrying
// tx's TxID via AttrItem.Tx — the seam app.nameservice uses to rebind a record's
// RegisteredAt to the controller-witnessed TxID.
func TestFoldBinding_ItemTxExposesWitnessedTxID(t *testing.T) {
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

// TestFoldBinding_BoundEditIDEqualsTxID pins the shared Go/C# invariant: a bound-path
// edit mints its EditID from the TxID with no parent seed, so EditID == TxID.  The C#
// FoldBinding.UpdateItem must satisfy the identical invariant (else the two clients
// derive different EditIDs on one wire and LWW winners diverge).
func TestFoldBinding_BoundEditIDEqualsTxID(t *testing.T) {
	node := tag.NowID()
	item := tag.NowID()
	txID := tag.NowID()

	binding := bindTag(node)
	gotEdit := tag.UID{}
	gotTx := tag.UID{}
	binding.OnItem = func(updated amp.AttrItem[*amp.Tag]) {
		gotEdit = updated.Addr.EditID
		if updated.Tx != nil {
			gotTx = updated.Tx.TxID()
		}
	}

	tx := amp.TxNew()
	tx.SetTxID(txID)
	if err := binding.UpsertItem(tx, item, &amp.Tag{Text: "x"}); err != nil {
		t.Fatal(err)
	}
	binding.OnNodeUpdate(amp.NodeUpdate{NodeID: node, Tx: tx})

	if gotEdit != txID {
		t.Fatalf("bound-path EditID = %v, want TxID %v (must mint from TxID, no parent seed)", gotEdit, txID)
	}
	if gotTx != txID {
		t.Fatalf("witnessed TxID = %v, want %v", gotTx, txID)
	}
}

// TestFoldBinding_DeleteFiresOnItemDeleted pins the delete branch the resolver's
// reverse-index maintenance relies on: a delete op is consulted by OnAdmit, fires
// OnItem with Deleted=true, and removes the live value.
func TestFoldBinding_DeleteFiresOnItemDeleted(t *testing.T) {
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

// nodeProbe counts one binding's callbacks: a FoldBinding read at item 0 or a
// LocusBinding read at cell 0, both of *amp.Tag.
type nodeProbe struct {
	responder amp.NodeResponder
	bind      func(nodeID tag.UID)
	boundTo   func() tag.UID
	value     func() (*amp.Tag, bool)
	items     int
	syncs     int
}

func newNodeProbe(attr tag.Name, locus bool) *nodeProbe {
	probe := &nodeProbe{}
	if locus {
		binding := amp.NewLocusBinding[*amp.Tag](attr)
		binding.OnCell = func(int, amp.AttrItem[*amp.Tag]) { probe.items++ }
		binding.OnSync = func() { probe.syncs++ }
		probe.responder = binding
		probe.bind = binding.Bind
		probe.boundTo = binding.NodeID
		probe.value = func() (*amp.Tag, bool) { return binding.GetCell(0) }
		return probe
	}
	binding := amp.NewFoldBinding[*amp.Tag](attr)
	binding.OnItem = func(amp.AttrItem[*amp.Tag]) { probe.items++ }
	binding.OnSync = func() { probe.syncs++ }
	probe.responder = binding
	probe.bind = binding.Bind
	probe.boundTo = binding.NodeID
	probe.value = func() (*amp.Tag, bool) { return binding.GetItem(tag.UID{}) }
	return probe
}

// offerNodes hands tx to the probe once per node, in order: the way a
// caller feeding bindings from one tx offers each node it touches.
func (probe *nodeProbe) offerNodes(tx *amp.TxMsg, nodeIDs ...tag.UID) {
	for _, nodeID := range nodeIDs {
		probe.responder.OnNodeUpdate(amp.NodeUpdate{
			NodeID: nodeID,
			Tx:     tx,
		})
	}
}

// expect fails unless the probe is bound to node and holds node's value,
// delivered by exactly one item callback and one sync callback.
func (probe *nodeProbe) expect(t *testing.T, node tag.UID) {
	t.Helper()
	if probe.boundTo() != node {
		t.Errorf("bound to %s, want %s", probe.boundTo(), node)
	}
	value, found := probe.value()
	if !found || tagText(value) != node.Base32() {
		t.Errorf("value = %q (found=%v), want %q", tagText(value), found, node.Base32())
	}
	if probe.items != 1 || probe.syncs != 1 {
		t.Errorf("callbacks: items=%d syncs=%d, want 1 each", probe.items, probe.syncs)
	}
}

// filterCase is one binding kind offered nodes A and B in one order.
type filterCase struct {
	name  string
	locus bool
	order []tag.UID
}

// filterFixture returns nodes A and B, a locus-base *amp.Tag attr, and the
// cases spanning both bindings that share acceptUpdateNode and both orders.
func filterFixture() (nodeA, nodeB tag.UID, attr tag.Name, cases []filterCase) {
	nodeA = tag.Name{}.With("test.bindings.node-a").ID
	nodeB = tag.Name{}.With("test.bindings.node-b").ID
	attr = tag.Name{}.With("test.bindings.locus.Tag")
	attr.ID = attr.ID.LocusBase()
	for _, locus := range []bool{false, true} {
		kind := "fold"
		if locus {
			kind = "locus"
		}
		cases = append(cases, filterCase{
			name:  kind + "/a-first",
			locus: locus,
			order: []tag.UID{nodeA, nodeB},
		}, filterCase{
			name:  kind + "/b-first",
			locus: locus,
			order: []tag.UID{nodeB, nodeA},
		})
	}
	return nodeA, nodeB, attr, cases
}

// nodesTx carries one upsert per node at (node, attrID, item 0), each valued
// with its node's base32.
func nodesTx(t *testing.T, attrID tag.UID, nodeIDs ...tag.UID) *amp.TxMsg {
	tx := amp.TxNew()
	tx.SetTxID(tag.NowID())
	for _, nodeID := range nodeIDs {
		value := &amp.Tag{Text: nodeID.Base32()}
		if err := tx.Upsert(nodeID, attrID, tag.UID{}, value); err != nil {
			t.Fatal(err)
		}
	}
	return tx
}

// TestBindings_BoundNodeRefusesOthers: a binding bound to A, offered a tx
// touching A and B in either order, keeps A's value with one item and one
// sync; B is refused whole, its sync included.
func TestBindings_BoundNodeRefusesOthers(t *testing.T) {
	nodeA, nodeB, attr, cases := filterFixture()
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			probe := newNodeProbe(attr, test.locus)
			probe.bind(nodeA)
			probe.offerNodes(nodesTx(t, attr.ID, nodeA, nodeB), test.order...)
			probe.expect(t, nodeA)
		})
	}
}

// TestBindings_UnboundBindsFirstNode: an unbound binding binds to the first
// node it is offered and refuses the next.
func TestBindings_UnboundBindsFirstNode(t *testing.T) {
	nodeA, nodeB, attr, cases := filterFixture()
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			probe := newNodeProbe(attr, test.locus)
			probe.offerNodes(nodesTx(t, attr.ID, nodeA, nodeB), test.order...)
			probe.expect(t, test.order[0])
		})
	}
}
