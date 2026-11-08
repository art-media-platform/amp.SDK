package std

import (
	"context"
	"fmt"
	"os"
	"path"
	"reflect"
	"time"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"github.com/art-media-platform/amp.SDK/stdlib/task"
	"google.golang.org/protobuf/proto"
)

// PushMetaOp sends a meta-op tx to the receiver for the client's session agent.
// Session control is not planet data: the tx carries no planet stamp
// (amp.PlanetTarget_None), so a receiving client tells a session push from a
// planet merge by envelope planet alone; an app pin's meta ops (MarshalMetaOp)
// carry the pin's planet.  guard bounds the push (dst.PushTx): the caller's
// concern — a request's submitter context, so a departed consumer cancels the
// push while a live one still receives it; sess itself when the push has no
// narrower owner.
func PushMetaOp(attrID tag.UID, value proto.Message, dst amp.TxReceiver, sess amp.Session, guard context.Context, contextID tag.UID, status amp.PinStatus) error {
	tx := sess.NewTx(amp.TxScope{Target: amp.PlanetTarget_None})
	tx.SetContextID(contextID)
	tx.Status = status

	if value != nil {
		head := tag.ElementID{
			NodeID: amp.HeadNodeID,
			AttrID: attrID,
		}
		if err := MarshalMetaOp(tx, head, value); err != nil {
			return err
		}
	}

	return dst.PushTx(tx, guard)
}

// PushSessionItem sends one item of a session index to the client's session
// agent: an upsert of value at (HeadNodeID, attrID, itemID), or a delete of
// that item when value is nil.  SessionIndex decides when an item may push;
// call it directly only after login is granted.
func PushSessionItem(sess amp.Session, attrID, itemID tag.UID, value proto.Message) error {
	tx := sess.NewTx(amp.TxScope{Target: amp.PlanetTarget_None})
	tx.SetContextID(SessionContextID)
	tx.Status = amp.PinStatus_Synced
	item := tag.ElementID{
		NodeID: amp.HeadNodeID,
		AttrID: attrID,
		ItemID: itemID,
	}
	if err := MarshalMetaOp(tx, item, value); err != nil {
		return err
	}
	return sess.PushTx(tx, sess)
}

// MarshalMetaOp appends one meta op at elem: an upsert of value, or a delete
// when value is nil.  A meta op is a host push a client receives and never
// stores (TxOpFlags_MetaOp, SD-edit-resolution §10 entry 24): session control
// on the session context, and an app pin's observations (a blob's progress
// ticks, AD-app-www §6.10) on the pin's own.  No commit carries one.
func MarshalMetaOp(tx *amp.TxMsg, elem tag.ElementID, value proto.Message) error {
	op := amp.TxOp{
		Flags: amp.TxOpFlags_Upsert | amp.TxOpFlags_MetaOp,
	}
	if value == nil {
		op.Flags = amp.TxOpFlags_Delete | amp.TxOpFlags_MetaOp
	}
	op.Addr.ElementID = elem
	return tx.MarshalOp(&op, value)
}

// LedgerNodeID is the well-known channel UID of the planet ledger — where
// Attestations (strikes, endorsements, witness records, audits, amnesties)
// are filed.  Deterministic across all planets so every member resolves the
// same channel for citation-based security workflows.
var LedgerNodeID = Attr.LedgerAttr.ID

// ArbitrateNodeID is the well-known channel UID of the planet arbitration channel —
// reserved for dispute proceedings, appeals, and formal rulings.  Peer institution
// to the ACC (which legislates) and the Ledger (which records the evidence).
var ArbitrateNodeID = Attr.ArbitrateAttr.ID

func PushSessionOp(sess amp.Session, attrID tag.UID, value proto.Message) error {
	return PushMetaOp(attrID, value, sess, sess, sess, SessionContextID, amp.PinStatus_Synced)
}

func ParseParamAsPath(req *amp.Request, paramKey string) (dirpath string, finfo os.FileInfo, err error) {
	var paramStr string
	err = req.ParseParam(paramKey, &paramStr)
	if err != nil {
		return
	}

	pathname := path.Clean(paramStr)
	finfo, err = os.Stat(pathname)
	if err != nil {
		err = status.Code_BadRequest.Errorf("param %q path error: %v", paramKey, err)
	}

	dirpath, _ = path.Split(pathname)
	return
}

func (root *ItemNode[AppT]) Root() *ItemNode[AppT] {
	return root
}

// PinAndServe creates a Pin, populates it via PinInto, serializes via MarshalAttrs, and pushes to the client.
// This is the standard entry point for send-only apps.
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

	_, err := app.StartChild(task.Task{
		Info: task.Info{
			Label:     label,
			IdleClose: time.Microsecond,
		},
		// pin.ctx is set before OnRun starts; PinInto and pushState read it.
		OnStart: func(pinContext task.Context) error {
			pin.ctx = pinContext
			return nil
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
				if err != status.ErrShuttingDown {
					pinContext.Log().Warnf("op failed: %v", err)
				}
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

// OnPinClosed is called when this Pin's task context is closed.
func (pin *Pin[AppT]) OnPinClosed() {
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

func (pin *Pin[AppT]) Context() task.Context {
	return pin.ctx
}

// pushState builds and sends the current state of all items to the client.
func (pin *Pin[AppT]) pushState() error {
	tx := pin.App.NewTx()

	{
		pinnedID := pin.Item.Root().ID
		w := itemWriter{
			tx:     tx,
			nodeID: pinnedID,
		}

		if err := tx.Upsert(amp.HeadNodeID, Attr.ChildLink.ID, pinnedID, nil); err != nil {
			return err
		}
		pin.Item.MarshalAttrs(&w)
		if w.err != nil {
			return w.err
		}

		for childID, child := range pin.children {
			w.nodeID = childID
			if err := tx.Upsert(pinnedID, Attr.ChildLink.ID, childID, nil); err != nil {
				return err
			}
			if childPin := childPinOf(child); childPin != nil {
				if err := tx.Upsert(pinnedID, Attr.ChildPin.ID, childID, childPin); err != nil {
					return err
				}
			}
			child.MarshalAttrs(&w)
			if w.err != nil {
				return w.err
			}
		}
	}

	tx.Status = amp.PinStatus_Synced
	return pin.Request.PushTx(tx, pin.ctx)
}

// childPinOf is the child's own pin URL, the ChildPin value; nil for an
// Item no pin of its own serves (its ChildLink cell alone lists it).
func childPinOf[AppT amp.AppInstance](child Item[AppT]) *amp.Tag {
	pinner, ok := child.(ChildPinner)
	if !ok {
		return nil
	}
	return pinner.ChildPin()
}

var _ ItemWriter = (*itemWriter)(nil)

type itemWriter struct {
	nodeID tag.UID
	tx     *amp.TxMsg
	err    error
}

func (w *itemWriter) PutTextAt(attrID, itemID tag.UID, value string) {
	if w.err != nil {
		return
	}
	err := w.tx.Upsert(w.nodeID, attrID, itemID, amp.TagText("text/plain", value))
	if err != nil {
		w.err = err
	}
}

func (w *itemWriter) PutItemAt(attrID, itemID tag.UID, value proto.Message) {
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

func (w *itemWriter) PutItem(attrID tag.UID, value proto.Message) {
	if value == nil {
		return
	}
	w.PutItemAt(attrID, tag.UID{}, value)
}
