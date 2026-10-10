package std

import (
	"slices"
	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// bindingProbe counts the callbacks of one fold or locus binding.
type bindingProbe struct {
	responder amp.NodeResponder
	value     func() (*amp.Tag, bool)
	updates   int
	syncs     int
}

func newBindingProbe(nodeID, itemID tag.UID, attr tag.Name, locus bool) *bindingProbe {
	probe := &bindingProbe{}
	if locus {
		binding := amp.NewLocusBinding[*amp.Tag](attr)
		binding.Bind(nodeID)
		binding.OnCell = func(cell int, item amp.AttrItem[*amp.Tag]) {
			probe.updates++
		}
		binding.OnSync = func() {
			probe.syncs++
		}
		probe.responder = binding
		probe.value = func() (*amp.Tag, bool) {
			return binding.GetCell(0)
		}
	} else {
		binding := amp.NewFoldBinding[*amp.Tag](attr)
		binding.Bind(nodeID)
		binding.OnItem = func(item amp.AttrItem[*amp.Tag]) {
			probe.updates++
		}
		binding.OnSync = func() {
			probe.syncs++
		}
		probe.responder = binding
		probe.value = func() (*amp.Tag, bool) {
			return binding.GetItem(itemID)
		}
	}
	return probe
}

// updateRecorder records the node of each update it receives.
type updateRecorder struct {
	nodes []tag.UID
}

func (recorder *updateRecorder) OnNodeUpdate(update amp.NodeUpdate) {
	recorder.nodes = append(recorder.nodes, update.NodeID)
}

func newNodesTx(t *testing.T, attrID tag.UID, nodeIDs ...tag.UID) *amp.TxMsg {
	tx := amp.TxNew()
	tx.SetTxID(tag.NowID())
	for _, nodeID := range nodeIDs {
		if err := tx.Upsert(nodeID, attrID, tag.UID{}, &amp.Tag{Text: nodeID.Base32()}); err != nil {
			t.Fatal(err)
		}
	}
	return tx
}

func TestPinMergeIncomingTypedBindingsByNode(t *testing.T) {
	nodeA := tag.Name{}.With("test.pin.node-a").ID
	nodeB := tag.Name{}.With("test.pin.node-b").ID
	attr := tag.Name{}.With("test.pin.locus.Tag")
	attr.ID = attr.ID.LocusBase()
	orders := []struct {
		name  string
		nodes []tag.UID
	}{
		{
			name:  "a-first",
			nodes: []tag.UID{nodeA, nodeB},
		},
		{
			name:  "b-first",
			nodes: []tag.UID{nodeB, nodeA},
		},
		{
			name:  "head-first",
			nodes: []tag.UID{amp.HeadNodeID, nodeA, nodeB},
		},
	}
	for _, locus := range []bool{false, true} {
		kind := "fold"
		if locus {
			kind = "locus"
		}
		for _, order := range orders {
			t.Run(kind+"/"+order.name, func(t *testing.T) {
				pin := &Pin[amp.AppInstance]{}
				probeA := newBindingProbe(nodeA, tag.UID{}, attr, locus)
				probeB := newBindingProbe(nodeB, tag.UID{}, attr, locus)
				pin.Bind(probeA.responder)
				pin.Bind(probeB.responder)
				pin.MergeIncoming(newNodesTx(t, attr.ID, order.nodes...))
				for idx, probe := range []*bindingProbe{probeA, probeB} {
					wantNode := []tag.UID{nodeA, nodeB}[idx]
					if wantNode == order.nodes[0] {
						continue
					}
					value, found := probe.value()
					if !found || value.Text != wantNode.Base32() {
						t.Errorf("node %s value = %v, found=%v", wantNode, value, found)
					}
					if probe.updates != 1 || probe.syncs != 1 {
						t.Errorf("node %s callbacks: updates=%d syncs=%d, want 1 each", wantNode, probe.updates, probe.syncs)
					}
				}
			})
		}
	}
}

func TestPinMergeIncomingCallbackAddsEarlierNode(t *testing.T) {
	nodeA := tag.Name{}.With("test.pin.dynamic-a").ID
	nodeB := tag.Name{}.With("test.pin.dynamic-b").ID
	attr := tag.Name{}.With("test.pin.dynamic.Tag")
	pin := &Pin[amp.AppInstance]{}
	driver := amp.NewFoldBinding[*amp.Tag](attr)
	driver.Bind(nodeB)
	added := newBindingProbe(nodeA, tag.UID{}, attr, false)
	driverCalls := 0
	driver.OnItem = func(item amp.AttrItem[*amp.Tag]) {
		driverCalls++
		pin.Bind(added.responder)
	}
	pin.Bind(driver)
	pin.MergeIncoming(newNodesTx(t, attr.ID, nodeA, nodeB))
	value, found := added.value()
	if !found || value.Text != nodeA.Base32() || driverCalls != 1 || added.updates != 1 || added.syncs != 1 {
		t.Fatalf("dynamic binding: value=%v found=%v driver=%d updates=%d syncs=%d", value, found, driverCalls, added.updates, added.syncs)
	}
}

// Each responder is offered each node once: one bound before the merge across
// the extra pass, and one bound during a callback, which that pass offers the
// whole tx.
func TestPinMergeIncomingOffersEachNodeOnce(t *testing.T) {
	nodeA := tag.Name{}.With("test.pin.once-a").ID
	nodeB := tag.Name{}.With("test.pin.once-b").ID
	attr := tag.Name{}.With("test.pin.once.Tag")
	pin := &Pin[amp.AppInstance]{}
	early := &updateRecorder{}
	late := &updateRecorder{}
	driver := amp.NewFoldBinding[*amp.Tag](attr)
	driver.Bind(nodeA)
	driver.OnItem = func(item amp.AttrItem[*amp.Tag]) {
		pin.Bind(late)
	}
	pin.Bind(early)
	pin.Bind(driver)
	pin.MergeIncoming(newNodesTx(t, attr.ID, nodeA, nodeB))
	want := []tag.UID{nodeA, nodeB}
	if !slices.Equal(early.nodes, want) {
		t.Errorf("pre-bound responder nodes = %v, want %v", early.nodes, want)
	}
	if !slices.Equal(late.nodes, want) {
		t.Errorf("callback-bound responder nodes = %v, want %v", late.nodes, want)
	}
}

// An unbound binding binds to the first node it is offered.
func TestPinMergeIncomingUnboundBinding(t *testing.T) {
	nodeID := tag.Name{}.With("test.pin.auto-bound").ID
	attr := tag.Name{}.With("test.pin.auto-bound.Tag")
	attr.ID = attr.ID.LocusBase()
	for _, locus := range []bool{false, true} {
		kind := "fold"
		if locus {
			kind = "locus"
		}
		t.Run(kind, func(t *testing.T) {
			probe := newBindingProbe(tag.UID{}, tag.UID{}, attr, locus)
			pin := &Pin[amp.AppInstance]{}
			pin.Bind(probe.responder)
			pin.MergeIncoming(newNodesTx(t, attr.ID, nodeID, amp.HeadNodeID))
			value, found := probe.value()
			if !found || value.Text != nodeID.Base32() || probe.updates != 1 || probe.syncs != 1 {
				t.Fatalf("auto-binding: value=%v found=%v updates=%d syncs=%d", value, found, probe.updates, probe.syncs)
			}
		})
	}
}
