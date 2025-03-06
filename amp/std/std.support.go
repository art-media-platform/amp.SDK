package std

import (
	"fmt"
	"reflect"
	"time"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"github.com/art-media-platform/amp.SDK/stdlib/task"
)

// Sends the single given value with attribute ID to the client's session agent for handling (e.g. LaunchOAuth)
func SendToClientAgent(sess amp.Session, attrID tag.ID, value tag.Value) error {
	return amp.SendMonoAttr(sess, attrID, value, ClientAgentID, amp.OpStatus_Synced)
}

func (root *CellNode[AppT]) Root() *CellNode[AppT] {
	return root
}

func PinAndServe[AppT amp.AppInstance](cell Cell[AppT], app AppT, op amp.Requester) (amp.Pin, error) {
	root := cell.Root()
	if root.ID.IsNil() {
		root.ID = tag.Now()
	}

	pin := &Pin[AppT]{
		Op:       op,
		App:      app,
		Cell:     cell,
		children: make(map[tag.ID]Cell[AppT]),
	}

	label := "pin: " + root.ID.Base32Suffix()
	if app.Info().DebugMode {
		label += fmt.Sprintf(", Cell.(*%v)", reflect.TypeOf(cell).Elem().Name())
	}

	var err error
	pin.ctx, err = app.StartChild(&task.Task{
		Info: task.Info{
			Label:     label,
			IdleClose: time.Microsecond,
		},
		OnRun: func(pinContext task.Context) {
			err := pin.App.MakeReady(op)
			if err == nil {
				err = cell.PinInto(pin)
			}
			if err == nil {
				err = pin.pushState()
			}
			if err != nil {
				if err != amp.ErrShuttingDown {
					pinContext.Log().Warnf("op failed: %v", err)
				}
			} else if op.Request().StateSync == amp.StateSync_Maintain {
				<-pinContext.Closing()
			}
			op.OnComplete(err)
		},
		OnClosing: func() {
			pin.ReleasePin()
		},
	})
	if err != nil {
		return nil, err
	}

	return pin, nil
}

func (app *App[AppT]) MakeReady(op amp.Requester) error {
	return nil
}

func (app *App[AppT]) PinAndServe(cell Cell[AppT], op amp.Requester) (amp.Pin, error) {
	return PinAndServe(cell, app.Instance, op)
}

func (app *App[AppT]) OnClosing() {
}

// Called when this Pin is closed.
// This allows a Cell to release resources it may locked during PinInto()..
func (pin *Pin[AppT]) ReleasePin() {
	// override for cleanup
}

func (pin *Pin[AppT]) AddChild(sub Cell[AppT]) {
	child := sub.Root()
	childID := child.ID
	if childID.IsNil() {
		childID = tag.Now()
		child.ID = childID
	}
	pin.children[childID] = sub
}

func (pin *Pin[AppT]) GetCell(target tag.ID) Cell[AppT] {
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

func (pin *Pin[AppT]) ServeRequest(op amp.Requester) (amp.Pin, error) {
	req := op.Request()
	cell := pin.GetCell(req.Select.CompositeID())
	if cell == nil {
		return nil, amp.ErrCellNotFound
	}
	return PinAndServe(cell, pin.App, op)
}

func (pin *Pin[AppT]) pushState() error {
	tx := amp.NewTxMsg(true)

	if pin.Op.Request().StateSync > amp.StateSync_None {
		pinnedID := pin.Cell.Root().ID

		w := cellWriter{
			tx:     tx,
			cellID: pinnedID,
		}

		tx.Upsert(amp.HeadCellID, CellChild.ID, pinnedID, nil) // publish root cell ID using the meta node
		pin.Cell.MarshalAttrs(&w)
		if w.err != nil {
			return w.err
		}

		for childID, child := range pin.children {
			w.cellID = childID
			tx.Upsert(pinnedID, CellChild.ID, childID, nil) // link child to pinned cell
			child.MarshalAttrs(&w)
			if w.err != nil {
				return w.err
			}
		}
	}

	tx.Status = amp.OpStatus_Synced
	return pin.Op.PushTx(tx)
}

type cellWriter struct {
	cellID tag.ID     // cache for Cell.Root().ID
	tx     *amp.TxMsg // in-progress transaction
	err    error
}

func (w *cellWriter) PushTextWithID(attrID, itemID tag.ID, value string) {
	if w.err != nil {
		return
	}
	op := amp.TxOp{}
	op.OpCode = amp.TxOpCode_UpsertElement
	op.CellID = w.cellID
	op.AttrID = attrID
	op.ItemID = itemID
	err := w.tx.MarshalOp(&op, &amp.Tag{
		Text: value,
	})
	if err != nil {
		w.err = err
	}
}

func (w *cellWriter) PushItemWithID(attrID, itemID tag.ID, value tag.Value) {
	if w.err != nil {
		return
	}
	op := amp.TxOp{}
	op.OpCode = amp.TxOpCode_UpsertElement
	op.CellID = w.cellID
	op.AttrID = attrID
	op.ItemID = itemID
	if err := w.tx.MarshalOp(&op, value); err != nil {
		w.err = err
	}
}

func (w *cellWriter) PushText(attrID tag.ID, value string) {
	if value == "" {
		return
	}
	w.PushTextWithID(attrID, tag.ID{}, value)
}

func (w *cellWriter) PushItem(attrID tag.ID, value tag.Value) {
	if value == nil {
		return
	}
	w.PushItemWithID(attrID, tag.ID{}, value)
}

func (w *cellWriter) Push(op *amp.TxOp, val tag.Value) {
	if w.err != nil {
		return
	}
	if err := w.tx.MarshalOp(op, val); err != nil {
		w.err = err
	}
}

/*
func (tx *TxMsg) PutMultiple(propertyIDs []tag.ID, serialize tag.Value) error {
	op := PropertyOp{}

	// serialize the value
	if serialize != nil {
		var err error
		op.DataOfs = uint64(len(tx.DataStore))
		tx.DataStore, err = serialize.MarshalToStore(tx.DataStore)
		if err != nil {
			tx.Error = err
			return err
		}
		op.DataLen = uint64(len(tx.DataStore)) - op.DataOfs
	}

	// add the op to the set
	for _, propID := range propertyIDs {
		op.PropertyID = propID
		tx.Ops = append(tx.Ops, op)
	}
	tx.OpsSorted = false
	return nil
}

func (tx *TxMsg) Upsert(literal any, keys ...tag.ID) {
	var tag *amp.Tag

	switch v := literal.(type) {
	case string:
		tag = &amp.Tag{
			InlineText: v,
			Use:        TagUse_Text,
		}
		break
	case *string:
		tag.InlineText = *v
		tag.Use = TagUse_Text
		break
	case *amp.Tag:
		tag = v
		break
	caseTag:
		tag = &v
		break
	// case *tag.ID:
	// 	tag = &amp.Tag{}
	// 	tag.SetTagUID(v)
	default:
		panic("unsupported type")
		return
	}

	for _, key := range keys {
		tx.Put(tag, key)
	}
}
*/
