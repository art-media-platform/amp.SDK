package std

import (
	"context"

	"time"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/closer"
	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"google.golang.org/protobuf/proto"
)

// Loads the latest element from the app instance associated with the current user; useful for storing high-level app state such as auth tokens.
func BlockingLoad(appCtx amp.AppContext, attrID tag.UID, dst proto.Message) error {
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
	if err != nil {
		return err
	}

	select {
	case err = <-req.outErr:
	case txOut := <-req.outTx:
		err = txOut.ExtractValue(attrID, tag.UID{}, dst)
	case <-appCtx.Closing():
		err = ctx.Err()
	}

	ctx.Close(err)
	return err
}

// Write version of BlockingLoad()
func BlockingStore(appCtx amp.AppContext, attrID tag.UID, src proto.Message) error {
	if src == nil {
		return status.ErrNothingToCommit
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
		URL:      AmpCabinetsURL,
	}
	req.Selector.Select(target)
	return req
}

type localLoad struct {
	outTx  chan *amp.TxMsg
	outErr chan error
}

func (req *localLoad) PushTx(tx *amp.TxMsg, ctx context.Context) error {
	if tx == nil {
		return status.ErrNothingToCommit
	}

	var err error

	select {
	case req.outTx <- tx:
	case <-ctx.Done():
		err = ctx.Err()
		req.outErr <- err
	}
	return err
}

// Commit submits tx and blocks until the commit completes.  The target planet is fixed at
// tx creation (NewTx via TxScope; default = the caller's home planet) — Commit does not alter
// it.  Planet routing, signer (tx.SetFromID), and privacy (tx.Epoch) are independent levers;
// see AOM SD-security-sync.md §7.6.
func Commit(appCtx amp.AppContext, tx *amp.TxMsg) error {
	tx.Request = &amp.PinRequest{
		Mode: amp.PinMode_Commit,
		URL:  AmpCabinetsURL,
	}

	req := &localCommit{
		Context:  appCtx,
		onCommit: make(chan error, 1), // buffered: a completion racing the timeout must never block its sender
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
	case <-time.After(CommitTimeout):
		err = status.Code_Timeout.Errorf("commit did not complete within %v — is the target planet attached to this session? (AOM O5 §5.11)", CommitTimeout)
	case <-appCtx.Closing():
		err = appCtx.Err()
	}

	ctx.Close(err)
	return err
}

// CommitTimeout bounds how long Commit blocks awaiting completion.  A commit
// that can never complete — e.g. its target planet is not attached to the
// session — otherwise hangs its app verb silently; the bound converts that
// class into a diagnosable error (AOM O5 §5.11).
var CommitTimeout = 30 * time.Second

type localCommit struct {
	context.Context
	onCommit chan error
}

func (req *localCommit) PushTx(tx *amp.TxMsg, ctx context.Context) error {
	return status.ErrInternal // never called
}

func (req *localCommit) RecvEvent(evt amp.PinEvent) {
	if evt.Status == amp.PinStatus_TxCommit {
		select {
		case req.onCommit <- evt.Error:
		default: // listener already timed out / left; drop rather than block the pipeline
		}
	}
}

// LoadItems loads a snapshot from the home cabinet and dispatches the result through a NodeResponder.
// This replaces manual TxMsg unwrapping patterns — the responder (typically an AttrBinding)
// receives typed callbacks for each matching item.
func LoadItems(appCtx amp.AppContext, nodeID tag.UID, attrID tag.UID, resp amp.NodeResponder) error {
	tx := appCtx.NewTx()
	tx.Request = SetupSnapshot(tag.ElementID{
		NodeID: nodeID,
		AttrID: attrID,
		ItemID: tag.WildcardID(),
	})

	loader := &localLoad{
		outTx:  make(chan *amp.TxMsg),
		outErr: make(chan error),
	}

	ctx := closer.WrapContext(appCtx)
	err := appCtx.Session().SubmitTx(amp.TxCommit{
		Tx:      tx,
		Origin:  loader,
		Context: ctx,
	})
	if err != nil {
		ctx.Close(err)
		return err
	}

	// A snapshot may arrive as MANY pushed txs — the serve splits on tx size,
	// and a Tape-class attr relays one envelope-carry tx per journal tx — so
	// drain until the terminal status (Synced; the host promotes the last
	// snapshot tx to Complete).
	for done := false; !done; {
		select {
		case err = <-loader.outErr:
			done = true
		case txOut := <-loader.outTx:
			resp.OnNodeUpdate(amp.NodeUpdate{
				Tx:       txOut,
				NodeID:   nodeID,
				Revision: tag.NowID(),
			})
			done = txOut.Status != amp.PinStatus_Syncing
		case <-appCtx.Closing():
			err = appCtx.Err()
			done = true
		}
	}

	ctx.Close(err)
	return err
}
