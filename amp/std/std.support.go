package std

import (
	"fmt"
	"os"
	"path"
	"reflect"
	"time"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"github.com/art-media-platform/amp.SDK/stdlib/task"
)

// Pushes a new tx to the receiver for the client's session agent for handling (e.g. LaunchOAuth)
// If value == nil, no op is marshalled and  the tx is sent without ops.
func PushMetaOp(attrID tag.UID, value amp.Value, dst amp.TxReceiver, sess amp.Session, contextID tag.UID, status amp.PinStatus) error {
	tx := sess.NewTx()
	tx.SetContextID(contextID)
	tx.Status = status

	if value != nil {
		op := amp.TxOp{
			Flags: amp.TxOpFlags_Upsert | amp.TxOpFlags_MetaOp,
		}
		op.Addr.NodeID = amp.HeadNodeID
		op.Addr.AttrID = attrID
		if err := tx.MarshalOp(&op, value); err != nil {
			return err
		}
	}

	return dst.PushTx(tx, sess)
}

var SessionContextID = tag.UID{0, 8675309} // symbolizes the client's session controller / agent.

// Convenience function for PushMetaOp()
func PushSessionOp(sess amp.Session, attrID tag.UID, value amp.Value) error {
	return PushMetaOp(attrID, value, sess, sess, SessionContextID, amp.PinStatus_Synced)
}

// Convenience function to parse the named URL value into the destinaation type
func ParseParamAsPath(req *amp.Request, paramKey string) (dirpath string, finfo os.FileInfo, err error) {
	var paramStr string
	err = req.ParseParam(paramKey, &paramStr)
	if err != nil {
		return
	}

	pathname := path.Clean(paramStr) // TODO: is this sufficent protection?
	finfo, err = os.Stat(pathname)
	if err != nil {
		err = amp.ErrCode_BadRequest.Errorf("param %q path error: %v", paramKey, err)
	}

	dirpath, _ = path.Split(pathname)
	return
}

func (root *ItemNode[AppT]) Root() *ItemNode[AppT] {
	return root
}

func PinAndServe[AppT amp.AppInstance](item Item[AppT], app AppT, req *amp.Request) (amp.Pin, error) {
	root := item.Root()
	if root.ID.IsNil() {
		root.ID = tag.NowID()
	}

	pin := &Pin[AppT]{
		Request:  req,
		App:      app,
		Item:     item,
		children: make(map[tag.UID]Item[AppT]),
	}

	label := "pin: " + root.ID.AsLabel()
	if app.Info().DebugMode {
		label += fmt.Sprintf(", Item.(*%v)", reflect.TypeOf(item).Elem().Name())
	}

	var err error
	pin.ctx, err = app.StartChild(task.Task{
		Info: task.Info{
			Label:     label,
			IdleClose: time.Microsecond,
		},
		OnRun: func(pinContext task.Context) {
			err := pin.App.MakeReady(req)
			if err == nil {
				err = item.PinInto(pin)
			}
			if err == nil {
				switch pin.Request.Current.Mode {
				case amp.PinMode_Snapshot, amp.PinMode_MaintainSync:
					err = pin.pushState()
				}
			}
			if err != nil {
				pin.fatal = err
				if err != amp.ErrShuttingDown {
					pinContext.Log().Warnf("op failed: %v", err)
				}
			}
			if err == nil && req.Current.Mode != amp.PinMode_Snapshot {
				// TODO: add tx submitter loop?
			}
			pin.Request.RecvEvent(amp.PinEvent{
				Status: amp.PinStatus_Complete,
				Error:  err,
			})
		},
		OnClosing: func() {
			pin.OnPinClosed()
		},
	})
	if err != nil {
		return nil, err
	}

	return pin, nil
}

func (app *AppModule[AppT]) MakeReady(req *amp.Request) error {
	return nil
}

func (app *AppModule[AppT]) PinAndServe(item Item[AppT], req *amp.Request) (amp.Pin, error) {
	return PinAndServe(item, app.Instance, req)
}

func (app *AppModule[AppT]) OnClosing() {
}

// Called when this Pin is closed.
// This allows a Item to release resources it may locked during PinInto()..
func (pin *Pin[AppT]) OnPinClosed() {
	// override for cleanup
}

func (pin *Pin[AppT]) AddChild(sub Item[AppT]) {
	child := sub.Root()
	childID := child.ID
	if childID.IsNil() {
		childID = tag.NowID()
		child.ID = childID
	}
	pin.children[childID] = sub
}

func (pin *Pin[AppT]) GetItem(target tag.UID) Item[AppT] {
	if target == pin.Item.Root().ID {
		return pin.Item
	}
	if item, exists := pin.children[target]; exists {
		return item
	}
	return nil
}

func (pin *Pin[AppT]) Context() task.Context {
	return pin.ctx
}

func (pin *Pin[AppT]) StartPin(req *amp.Request) (amp.Pin, error) {
	if err := req.ParseAsAddressURL(); err == nil {
		return nil, err
	}

	targetID := req.Selector.Spans[0].NodeID() // TODO: is this the best?

	item := pin.GetItem(targetID)
	if item == nil {
		return nil, amp.ErrItemNotFound
	}
	return PinAndServe(item, pin.App, req)
}

func (pin *Pin[AppT]) CommitTx(tx *amp.TxMsg) error {
	// tx.AddRef()
	return amp.ErrUnimplemented
}

func (pin *Pin[AppT]) ReviseRequest(latest *amp.PinRequest) error {
	return nil
}

// Pushes a TxMsg to the client of this Pin, pushing the state of each item and its children.
func (pin *Pin[AppT]) pushState() error {
	tx := pin.App.NewTx()

	{
		pinnedID := pin.Item.Root().ID
		w := itemWriter{
			tx:     tx,
			nodeID: pinnedID,
		}

		tx.Upsert(amp.HeadNodeID, ChildLink, pinnedID, nil) // publish root pinned item
		pin.Item.MarshalAttrs(&w)
		if w.err != nil {
			return w.err
		}

		for childID, child := range pin.children {
			w.nodeID = childID
			tx.Upsert(pinnedID, ChildLink, childID, nil) // link child to pinned item
			child.MarshalAttrs(&w)
			if w.err != nil {
				return w.err
			}
		}
	}

	tx.Status = amp.PinStatus_Synced
	return pin.Request.PushTx(tx, pin.ctx)
}

var _ ItemWriter = (*itemWriter)(nil)

type itemWriter struct {
	nodeID tag.UID    // cache for Item.Root().ID
	tx     *amp.TxMsg // in-progress transaction
	err    error
}

func (w *itemWriter) PutTextAt(attrID, itemID tag.UID, value string) {
	if w.err != nil {
		return
	}
	err := w.tx.Upsert(w.nodeID, attrID, itemID, &TextItem{
		Body: value,
	})
	if err != nil {
		w.err = err
	}
}

func (w *itemWriter) PutItemAt(attrID, itemID tag.UID, value amp.Value) {
	if w.err != nil {
		return
	}
	if err := w.tx.Upsert(w.nodeID, attrID, itemID, value); err != nil {
		w.err = err
	}
}

func (w *itemWriter) PutText(attrID tag.UID, value string) {
	if value == "" {
		return
	}
	w.PutTextAt(attrID, tag.UID{}, value)
}

func (w *itemWriter) PutItem(attrID tag.UID, value amp.Value) {
	if value == nil {
		return
	}
	w.PutItemAt(attrID, tag.UID{}, value)
}

func (v *TextItem) New() amp.Value {
	return &TextItem{}
}

func (v *TextItem) MarshalToStore(in []byte) (out []byte, err error) {
	return amp.MarshalPbToStore(v, in)
}

func (v *Report) New() amp.Value {
	return &Report{}
}

func (v *Report) MarshalToStore(in []byte) (out []byte, err error) {
	return amp.MarshalPbToStore(v, in)
}
