package std

import (
	"context"
	"fmt"
	"os"
	"path"
	"reflect"
	"time"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"github.com/art-media-platform/amp.SDK/stdlib/task"
)

var SessionContextID = tag.UID{0, 8675309}

// Sends the given attr-value pair to the client's session agent for handling (e.g. LaunchOAuth)
func PushSessionOp(sess amp.Session, attrID tag.UID, value amp.Value) error {
	kv := amp.ValueEntry{
		Value: value,
	}
	kv.Addr.ChanID = amp.HeadChannelID
	kv.Addr.AttrID = attrID
	return PushMetaOp(sess, sess, kv, SessionContextID, amp.PinStatus_Synced)
}

// PushMetaOp sends a TxOp to the given destination with the given value.
func PushMetaOp(dst amp.TxReceiver, ctx context.Context, kv amp.ValueEntry, contextID tag.UID, status amp.PinStatus) error {
	op := amp.TxOp{
		OpCode: amp.TxOpCode_MetaOp,
		Addr:   kv.Addr,
	}

	tx := amp.TxGenesis()
	tx.SetContextID(contextID)
	tx.Status = status
	if err := tx.MarshalOp(&op, kv.Value); err != nil {
		return err
	}

	return dst.PushTx(tx, ctx)
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

func (root *CellNode[AppT]) Root() *CellNode[AppT] {
	return root
}

func PinAndServe[AppT amp.AppInstance](cell Cell[AppT], app AppT, req *amp.Request) (amp.Pin, error) {
	root := cell.Root()
	if root.ID.IsNil() {
		root.ID = tag.UID_Now()
	}

	pin := &Pin[AppT]{
		Request:  req,
		App:      app,
		Cell:     cell,
		children: make(map[tag.UID]Cell[AppT]),
	}

	label := "pin: " + root.ID.AsLabel()
	if app.Info().DebugMode {
		label += fmt.Sprintf(", Cell.(*%v)", reflect.TypeOf(cell).Elem().Name())
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
				err = cell.PinInto(pin)
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

func (app *AppModule[AppT]) PinAndServe(cell Cell[AppT], req *amp.Request) (amp.Pin, error) {
	return PinAndServe(cell, app.Instance, req)
}

func (app *AppModule[AppT]) OnClosing() {
}

// Called when this Pin is closed.
// This allows a Cell to release resources it may locked during PinInto()..
func (pin *Pin[AppT]) OnPinClosed() {
	// override for cleanup
}

func (pin *Pin[AppT]) AddChild(sub Cell[AppT]) {
	child := sub.Root()
	childID := child.ID
	if childID.IsNil() {
		childID = tag.UID_Now()
		child.ID = childID
	}
	pin.children[childID] = sub
}

func (pin *Pin[AppT]) GetCell(target tag.UID) Cell[AppT] {
	if target == pin.Cell.Root().ID {
		return pin.Cell
	}
	if cell, exists := pin.children[target]; exists {
		return cell
	}
	return nil
}

func (pin *Pin[AppT]) Context() task.Context {
	return pin.ctx
}

func (pin *Pin[AppT]) StartPin(req *amp.Request) (amp.Pin, error) {
	invokeTag := req.Current.Invoke
	targetID := invokeTag.UID()

	cell := pin.GetCell(targetID)
	if cell == nil {
		return nil, amp.ErrChannelNotFound
	}
	return PinAndServe(cell, pin.App, req)
}

func (pin *Pin[AppT]) CommitTx(tx *amp.TxMsg) error {
	// tx.AddRef()
	return amp.ErrUnimplemented
}

func (pin *Pin[AppT]) ReviseRequest(latest *amp.PinRequest) error {
	return amp.ErrUnimplemented // TODO
}

// Pushes a TxMsg to the client of this Pin, pushing the state of each cell and its children.
func (pin *Pin[AppT]) pushState() error {
	tx := amp.TxGenesis()

	{
		pinnedID := pin.Cell.Root().ID
		w := cellWriter{
			tx:     tx,
			chanID: pinnedID,
		}

		tx.Upsert(amp.HeadChannelID, ItemIndex, pinnedID, nil) // publish root cell ID using the meta node
		pin.Cell.MarshalAttrs(&w)
		if w.err != nil {
			return w.err
		}

		for childID, child := range pin.children {
			w.chanID = childID
			tx.Upsert(pinnedID, ItemIndex, childID, nil) // link child to pinned cell
			child.MarshalAttrs(&w)
			if w.err != nil {
				return w.err
			}
		}
	}

	tx.Status = amp.PinStatus_Synced
	return pin.Request.PushTx(tx, pin.ctx)
}

var _ CellWriter = (*cellWriter)(nil)

type cellWriter struct {
	chanID tag.UID    // cache for Cell.Root().ID
	tx     *amp.TxMsg // in-progress transaction
	err    error
}

func (w *cellWriter) PushTextWithID(attrID, itemID tag.UID, value string) {
	if w.err != nil {
		return
	}
	op := amp.TxOp{}
	op.OpCode = amp.TxOpCode_Upsert
	op.Addr.ChanID = w.chanID
	op.Addr.AttrID = attrID
	op.Addr.ItemID = itemID
	err := w.tx.MarshalOp(&op, &amp.Tag{
		Text: value,
	})
	if err != nil {
		w.err = err
	}
}

func (w *cellWriter) PushItemWithID(attrID, itemID tag.UID, value amp.Value) {
	if w.err != nil {
		return
	}
	op := amp.TxOp{}
	op.OpCode = amp.TxOpCode_Upsert
	op.Addr.ChanID = w.chanID
	op.Addr.AttrID = attrID
	op.Addr.ItemID = itemID
	if err := w.tx.MarshalOp(&op, value); err != nil {
		w.err = err
	}
}

func (w *cellWriter) PushText(attrID tag.UID, value string) {
	if value == "" {
		return
	}
	w.PushTextWithID(attrID, tag.UID{}, value)
}

func (w *cellWriter) PushItem(attrID tag.UID, value amp.Value) {
	if value == nil {
		return
	}
	w.PushItemWithID(attrID, tag.UID{}, value)
}

func (w *cellWriter) Push(op *amp.TxOp, val amp.Value) {
	if w.err != nil {
		return
	}
	if err := w.tx.MarshalOp(op, val); err != nil {
		w.err = err
	}
}
