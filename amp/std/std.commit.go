package std

import (
	"context"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/closer"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// Loads the latest element from the app instance associated with the current user; useful for storing high-level app state such as auth tokens.
func BlockingLoad(appCtx amp.AppContext, attrID tag.UID, dst amp.Value) error {
	appEnv := appCtx.AppEnvironment()

	tx := appCtx.NewTx()
	tx.Request = SetupSnapshot(tag.ElementID{
		NodeID: appEnv.HomeID,
		AttrID: attrID,
	})

	req := &localLoad{
		outTx:  make(chan *amp.TxMsg),
		outErr: make(chan error),
	}

	ctx := closer.WrapContext(appCtx)
	err := appCtx.Session().SubmitTx(amp.TxCommit{
		Tx:      tx,
		Origin:  req,
		Context: ctx,
	})
	tx.ReleaseRef()
	if err != nil {
		return err
	}

	select {
	case err = <-req.outErr:
	case txOut := <-req.outTx:
		err = txOut.ExtractValue(attrID, tag.UID{}, dst)
		txOut.ReleaseRef()
	case <-appCtx.Closing():
		err = ctx.Err()
	}

	ctx.Close(err)
	return err
}

// Write version of BlockingLoad()
func BlockingStore(appCtx amp.AppContext, attrID tag.UID, src amp.Value) error {
	if src == nil {
		return amp.ErrNothingToCommit
	}

	appEnv := appCtx.AppEnvironment()
	itemID := tag.UID{}

	tx := appCtx.NewTx()
	if err := tx.Upsert(appEnv.HomeID, attrID, itemID, src); err != nil {
		return err
	}

	err := Commit(appCtx, tx)
	return err
}

func SetupSnapshot(target tag.ElementID) *amp.PinRequest {
	req := &amp.PinRequest{
		Mode:     amp.PinMode_Snapshot,
		Selector: &amp.ItemSelector{},
		Invoke: &amp.Tag{
			URI: "amp://cabinets/~",
		},
	}
	req.Selector.Select(target)
	return req
}

func SetupCommit(appCtx amp.AppContext, tx *amp.TxMsg) {
	tx.TxHeader.Recipient = appCtx.Session().Login().Planet
	tx.TxHeader.Request = &amp.PinRequest{
		Mode: amp.PinMode_Commit,
		Invoke: &amp.Tag{
			URI: "amp://cabinets/~",
		},
	}
}

type localLoad struct {
	outTx  chan *amp.TxMsg
	outErr chan error
}

func (req *localLoad) PushTx(tx *amp.TxMsg, ctx context.Context) error {
	if tx == nil {
		return amp.ErrNothingToCommit
	}

	tx.AddRef()
	var err error

	select {
	case req.outTx <- tx:
	case <-ctx.Done():
		err = ctx.Err()
		req.outErr <- err
	}
	return err
}

// func (req *localLoad) RecvEvent(evt amp.PinEvent) {
// 	// not used
// }

func Commit(appCtx amp.AppContext, tx *amp.TxMsg) error {
	SetupCommit(appCtx, tx)

	req := &localCommit{
		Context:  appCtx,
		onCommit: make(chan error),
	}

	ctx := closer.WrapContext(appCtx)
	err := appCtx.Session().SubmitTx(amp.TxCommit{
		Tx:      tx,
		Origin:  req,
		Context: ctx,
	})
	if err != nil {
		return err
	}

	select {
	case err = <-req.onCommit:
	case <-appCtx.Closing():
		err = appCtx.Err()
	}

	ctx.Close(err)
	return err
}

type localCommit struct {
	context.Context
	onCommit chan error
}

func (req *localCommit) PushTx(tx *amp.TxMsg, ctx context.Context) error {
	return amp.ErrInternal // never called
}

func (req *localCommit) RecvEvent(evt amp.PinEvent) { // TODO delete this or move RecvEvent in TxReceiver
	if evt.Status == amp.PinStatus_TxCommit {
		req.onCommit <- evt.Error
	}
}

/*

func (tx *TxMsg) Upsert(literal any, keys ...tag.UID) {
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
	// case *tag.UID:
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
