package std

import (
	"context"
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

// linkedItem is a child a pin of its own serves (ChildLinker).
type linkedItem struct {
	ItemNode[*joiningApp]
	url string
}

func (item *linkedItem) PinInto(*Pin[*joiningApp]) error { return nil }
func (item *linkedItem) MarshalAttrs(ItemWriter)         {}
func (item *linkedItem) ChildLink() *amp.Tag {
	return &amp.Tag{
		URI: item.url,
	}
}

// listingItem lists one linked child and one plain child.
type listingItem struct {
	ItemNode[*joiningApp]
	linked *linkedItem
	plain  *contextItem
}

func (item *listingItem) PinInto(pin *Pin[*joiningApp]) error {
	pin.AddChild(item.linked)
	pin.AddChild(item.plain)
	return nil
}

func (item *listingItem) MarshalAttrs(ItemWriter) {}

// pushRequester keeps the tx a push carries.
type pushRequester struct {
	tx *amp.TxMsg
}

func (requester *pushRequester) PushTx(tx *amp.TxMsg, _ context.Context) error {
	requester.tx = tx
	return nil
}

func (requester *pushRequester) RecvEvent(amp.PinEvent) {}

// A listing's ChildLink cell carries a ChildLinker child's own pin URL; a
// plain child's stays a presence cell (MD-unity-client §3.11).
func TestPushStateChildLink(t *testing.T) {
	parent, err := task.NewChild(nil, "child-link-parent")
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
	item := &listingItem{
		linked: &linkedItem{url: "amp://~/app.filesys.posix?path=%2Fmusic%2Fa.aac"},
		plain:  &contextItem{},
	}
	item.linked.ID = tag.NowID()
	item.plain.ID = tag.NowID()
	requester := &pushRequester{}
	_, err = PinAndServe(item, app, &amp.Request{
		Requester: requester,
		Current:   amp.PinRequest{Mode: amp.PinMode_Snapshot},
	})
	if err != nil {
		t.Fatal(err)
	}
	if requester.tx == nil {
		t.Fatal("no state pushed")
	}
	link := &amp.Tag{}
	if err := requester.tx.ExtractValue(Attr.ChildLink.ID, item.linked.ID, link); err != nil {
		t.Fatalf("linked child's ChildLink carries no value: %v", err)
	}
	if link.URI != item.linked.url {
		t.Errorf("linked child's ChildLink URI = %q, want %q", link.URI, item.linked.url)
	}
	plain := &amp.Tag{}
	if err := requester.tx.ExtractValue(Attr.ChildLink.ID, item.plain.ID, plain); err != nil {
		t.Fatalf("plain child's ChildLink cell missing: %v", err)
	}
	if plain.URI != "" {
		t.Errorf("plain child's ChildLink URI = %q, want none (a presence cell)", plain.URI)
	}
}
