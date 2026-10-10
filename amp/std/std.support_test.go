package std

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"github.com/art-media-platform/amp.SDK/stdlib/task"
)

// stubApp refuses every child task and counts the attempts.
type stubApp struct {
	App
	childStarts int
}

func (app *stubApp) StartPin(req *amp.Request) (amp.Pin, error) {
	return nil, status.ErrShuttingDown
}

func (app *stubApp) Info() task.Info {
	return task.Info{}
}

func (app *stubApp) StartChild(child task.Task) (task.Context, error) {
	app.childStarts++
	return nil, status.ErrShuttingDown
}

type stubItem struct {
	ItemNode[*stubApp]
}

func (item *stubItem) PinInto(pin *Pin[*stubApp]) error {
	return nil
}

func (item *stubItem) MarshalAttrs(writer ItemWriter) {}

func TestPinStartPinSelectors(t *testing.T) {
	rootID := tag.Name{}.With("test.pin.root").ID
	childID := tag.Name{}.With("test.pin.child").ID
	missingID := tag.Name{}.With("test.pin.missing").ID
	cases := []struct {
		name       string
		invokeURL  string
		selector   tag.UID
		wantError  error
		wantCode   status.Code
		wantStarts int
	}{
		{
			name:     "missing-url",
			wantCode: status.Code_BadRequest,
		},
		{
			name:      "missing-selector",
			invokeURL: "amp://~/example",
			wantCode:  status.Code_BadRequest,
		},
		{
			name:      "unknown-child",
			invokeURL: "amp://~/example/" + missingID.Base32(),
			wantError: status.ErrItemNotFound,
		},
		{
			name:       "root",
			invokeURL:  "amp://~/example/" + rootID.Base32(),
			wantError:  status.ErrShuttingDown,
			wantStarts: 1,
		},
		{
			name:       "child",
			invokeURL:  "amp://~/example/" + childID.Base32(),
			wantError:  status.ErrShuttingDown,
			wantStarts: 1,
		},
		{
			name:       "explicit-selector",
			invokeURL:  "amp://~/example",
			selector:   childID,
			wantError:  status.ErrShuttingDown,
			wantStarts: 1,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			app := &stubApp{}
			pin := &Pin[*stubApp]{
				App:  app,
				Item: &stubItem{ItemNode: ItemNode[*stubApp]{ID: rootID}},
				children: map[tag.UID]Item[*stubApp]{
					childID: &stubItem{ItemNode: ItemNode[*stubApp]{ID: childID}},
				},
			}
			req := &amp.Request{}
			if test.invokeURL != "" {
				invokeURL, err := url.Parse(test.invokeURL)
				if err != nil {
					t.Fatal(err)
				}
				req.InvokeURL = invokeURL
			}
			if test.selector.IsSet() {
				req.Selector.Select(tag.ElementID{NodeID: test.selector})
			}
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Errorf("StartPin panicked: %v", recovered)
				}
			}()
			_, err := pin.StartPin(req)
			if test.wantError != nil && err != test.wantError {
				t.Errorf("error = %v, want %v", err, test.wantError)
			}
			if test.wantCode != 0 && status.CodeOf(err, status.Code_Unnamed) != test.wantCode {
				t.Errorf("error = %v, want code %v", err, test.wantCode)
			}
			if test.wantStarts > 0 && app.childStarts != test.wantStarts {
				t.Errorf("child starts = %d, want %d", app.childStarts, test.wantStarts)
			}
		})
	}
}

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
