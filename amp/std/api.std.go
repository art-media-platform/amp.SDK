// Package std provides Go support classes for building amp apps.
//
// Most amp apps embed App.  An app serves state through the send-only
// Item/PinAndServe pattern; it reads typed planet state through an
// amp.FoldBinding fed by LoadItems.
package std

import (
	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"github.com/art-media-platform/amp.SDK/stdlib/task"
	"google.golang.org/protobuf/proto"
)

// App is the base every amp app embeds: it carries the runtime AppContext and
// supplies no-op MakeReady / OnClosing, so an app implements only StartPin
// (and overrides the lifecycle hooks it actually uses).
type App struct {
	amp.AppContext
}

func (app *App) MakeReady(req *amp.Request) error { return nil }
func (app *App) OnClosing()                       {}

// Item is how std implements a send-only node for serving state to clients.
// Apps that only push state (filesys, spotify, tunr) implement this interface.
type Item[AppT amp.AppInstance] interface {
	Root() *ItemNode[AppT]

	// PinInto is called to populate this item (load children, fetch data, etc).
	PinInto(dst *Pin[AppT]) error

	// MarshalAttrs serializes this item's attributes into a tx for the client.
	MarshalAttrs(w ItemWriter)
}

// ItemNode is a base struct for Item implementations — holds a stable node ID.
type ItemNode[AppT amp.AppInstance] struct {
	ID tag.UID
}

// Pin serves one request send-only: PinAndServe populates the pinned item
// and its children via PinInto, serializes them via MarshalAttrs, pushes that
// state once (Snapshot or MaintainSync), then completes.  A Pin takes no
// writes: it is not an amp.PinCommitter.
type Pin[AppT amp.AppInstance] struct {
	Request *amp.Request // originating request
	Item    Item[AppT]   // pinned item
	App     AppT         // parent app instance
	Sync    amp.PinMode  // Op.Request().PinMode

	fatal    error                  // fatal error, if any
	children map[tag.UID]Item[AppT] // child items
	ctx      task.Context           // task context for this pin
}

// ItemWriter serializes item attributes into a transaction.
type ItemWriter interface {
	PutTextAt(attrID, itemID tag.UID, value string)
	PutItemAt(attrID, itemID tag.UID, value proto.Message)
	PutText(attrID tag.UID, value string)
	PutItem(attrID tag.UID, value proto.Message)
}

const (
	ResourcesURL = "file://_resources_/"
)
