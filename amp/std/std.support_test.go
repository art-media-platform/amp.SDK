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
