package std

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"github.com/art-media-platform/amp.SDK/stdlib/task"
)

// joiningApp starts real child tasks but returns from StartChild only after
// OnRun returns: an ordering task.StartChild permits, made certain here.
type joiningApp struct {
	amp.AppInstance
	parent task.Context
}

func (app *joiningApp) Info() task.Info {
	return app.parent.Info()
}

func (app *joiningApp) MakeReady(*amp.Request) error {
	return nil
}

func (app *joiningApp) NewTx(...amp.TxScope) *amp.TxMsg {
	tx := amp.TxNew()
	tx.SetTxID(tag.NowID())
	return tx
}

func (app *joiningApp) StartChild(spec task.Task) (task.Context, error) {
	complete := make(chan struct{})
	run := spec.OnRun
	spec.OnRun = func(ctx task.Context) {
		defer close(complete)
		run(ctx)
	}
	child, err := app.parent.StartChild(spec)
	if err == nil {
		<-complete
	}
	return child, err
}

// contextItem records the pin context PinInto sees.
type contextItem struct {
	ItemNode[*joiningApp]
	observed task.Context
}

func (item *contextItem) PinInto(pin *Pin[*joiningApp]) error {
	item.observed = pin.Context()
	return nil
}

func (item *contextItem) MarshalAttrs(ItemWriter) {}

// guardRequester records the guard a push carries.
type guardRequester struct {
	guard context.Context
}

func (requester *guardRequester) PushTx(_ *amp.TxMsg, guard context.Context) error {
	requester.guard = guard
	return nil
}

func (requester *guardRequester) RecvEvent(amp.PinEvent) {}

func TestPinContextBeforePopulationAndPush(t *testing.T) {
	parent, err := task.NewChild(nil, "pin-context-parent")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		parent.Close()
		select {
		case <-parent.Done():
		case <-time.After(time.Second):
			t.Error("parent failed to drain")
		}
	})
	app := &joiningApp{
		parent: parent,
	}
	item := &contextItem{}
	requester := &guardRequester{}
	pin, err := PinAndServe(item, app, &amp.Request{
		Requester: requester,
		Current:   amp.PinRequest{Mode: amp.PinMode_Snapshot},
	})
	if err != nil {
		t.Fatal(err)
	}
	if item.observed == nil || item.observed != pin.Context() {
		t.Errorf("PinInto context missing=%v differs=%v", item.observed == nil, item.observed != pin.Context())
	}
	if requester.guard == nil || requester.guard != pin.Context() {
		t.Errorf("PushTx guard missing=%v differs=%v", requester.guard == nil, requester.guard != pin.Context())
	}
}

// pinnedItem is a child a pin of its own serves (ChildPinner); pin is what
// ChildPin() answers.
type pinnedItem struct {
	ItemNode[*joiningApp]
	pin *amp.Tag
}

func (item *pinnedItem) PinInto(*Pin[*joiningApp]) error { return nil }
func (item *pinnedItem) MarshalAttrs(ItemWriter)         {}
func (item *pinnedItem) ChildPin() *amp.Tag {
	return item.pin
}

// listingItem lists its children.
type listingItem struct {
	ItemNode[*joiningApp]
	children []Item[*joiningApp]
}

func (item *listingItem) PinInto(pin *Pin[*joiningApp]) error {
	for _, child := range item.children {
		pin.AddChild(child)
	}
	return nil
}

func (item *listingItem) MarshalAttrs(ItemWriter) {}

// pushRequester keeps the tx a push carries and the pin's completion.
type pushRequester struct {
	tx       *amp.TxMsg
	complete *amp.PinEvent
}

func (requester *pushRequester) PushTx(tx *amp.TxMsg, _ context.Context) error {
	requester.tx = tx
	return nil
}

func (requester *pushRequester) RecvEvent(evt amp.PinEvent) {
	if evt.Status == amp.PinStatus_Complete {
		requester.complete = &evt
	}
}

// serveListing serves item as a Snapshot pin and returns what it pushed and
// how it completed (joiningApp runs the pin to completion inside PinAndServe).
func serveListing(t *testing.T, item *listingItem) *pushRequester {
	t.Helper()
	parent, err := task.NewChild(nil, "child-pin-parent")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		parent.Close()
		select {
		case <-parent.Done():
		case <-time.After(time.Second):
			t.Error("parent failed to drain")
		}
	})
	app := &joiningApp{
		parent: parent,
	}
	requester := &pushRequester{}
	_, err = PinAndServe(item, app, &amp.Request{
		Requester: requester,
		Current:   amp.PinRequest{Mode: amp.PinMode_Snapshot},
	})
	if err != nil {
		t.Fatal(err)
	}
	if requester.complete == nil {
		t.Fatal("the pin did not complete")
	}
	return requester
}

// childLinkValueLens is the value byte length of every ChildLink cell at
// (nodeID, ChildLink, itemID) in tx.
func childLinkValueLens(t *testing.T, tx *amp.TxMsg, nodeID, itemID tag.UID) []int {
	t.Helper()
	lens := []int{}
	for idx, op := range tx.Ops {
		if op.Addr.NodeID != nodeID || op.Addr.AttrID != Attr.ChildLink.ID || op.Addr.ItemID != itemID {
			continue
		}
		raw, err := tx.OpValueBytes(idx)
		if err != nil {
			t.Fatalf("ChildLink op %d: %v", idx, err)
		}
		lens = append(lens, len(raw))
	}
	return lens
}

// A listing's ChildPin cell carries a ChildPinner child's own pin URL beside a
// value-less ChildLink; a ChildPinner answering nil and a plain child have the
// ChildLink alone; the head's ChildLink naming the pinned item is value-less
// too (ZO §4.8; AD-playlists §6; MD-unity-client §3.11).
func TestPushStateChildLink(t *testing.T) {
	url := "amp://~/app.filesys.posix?path=%2Fmusic%2Fa.aac"
	pinned := &pinnedItem{
		pin: &amp.Tag{URI: url},
	}
	unpinned := &pinnedItem{}
	plain := &contextItem{}
	pinned.ID = tag.NowID()
	unpinned.ID = tag.NowID()
	plain.ID = tag.NowID()
	item := &listingItem{
		children: []Item[*joiningApp]{pinned, unpinned, plain},
	}
	item.ID = tag.NowID()
	requester := serveListing(t, item)
	if requester.complete.Error != nil {
		t.Fatalf("listing completed with %v", requester.complete.Error)
	}
	if requester.tx == nil {
		t.Fatal("no state pushed")
	}
	tx := requester.tx
	childPin := &amp.Tag{}
	if err := tx.ExtractValue(Attr.ChildPin.ID, pinned.ID, childPin); err != nil {
		t.Fatalf("pinned child's ChildPin cell missing: %v", err)
	}
	if childPin.URI != url {
		t.Errorf("pinned child's ChildPin URI = %q, want %q", childPin.URI, url)
	}
	if err := tx.ExtractValue(Attr.ChildPin.ID, unpinned.ID, &amp.Tag{}); err == nil {
		t.Error("a ChildPinner answering nil carries a ChildPin cell, want none")
	}
	if err := tx.ExtractValue(Attr.ChildPin.ID, plain.ID, &amp.Tag{}); err == nil {
		t.Error("plain child carries a ChildPin cell, want none")
	}
	if lens := childLinkValueLens(t, tx, amp.HeadNodeID, item.ID); len(lens) != 1 || lens[0] != 0 {
		t.Errorf("head ChildLink value byte lengths %v, want one value-less set marker", lens)
	}
	for _, child := range []tag.UID{pinned.ID, unpinned.ID, plain.ID} {
		if lens := childLinkValueLens(t, tx, item.ID, child); len(lens) != 1 || lens[0] != 0 {
			t.Errorf("child %s: ChildLink value byte lengths %v, want one value-less set marker", child.Base32(), lens)
		}
	}
}

// A ChildPin the tx refuses (Tag.Validate: Data with no ContentTypeRaw) fails
// the listing: pushState returns the Upsert error and nothing is pushed.
func TestPushStateChildPinRefused(t *testing.T) {
	refused := &pinnedItem{
		pin: &amp.Tag{Data: []byte("no content type")},
	}
	refused.ID = tag.NowID()
	item := &listingItem{
		children: []Item[*joiningApp]{refused},
	}
	item.ID = tag.NowID()
	requester := serveListing(t, item)
	if !errors.Is(requester.complete.Error, amp.ErrTagNoContentType) {
		t.Errorf("listing completed with %v, want %v", requester.complete.Error, amp.ErrTagNoContentType)
	}
	if requester.tx != nil {
		t.Errorf("a refused listing pushed %d ops", len(requester.tx.Ops))
	}
}
