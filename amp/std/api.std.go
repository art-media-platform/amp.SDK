// A collection of types and functions that form a support template when building an amp.App.
// Most amp.Apps build upon this template but a specialized app may opt to build their own foundation upon the amp api.
package std

import (
	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"github.com/art-media-platform/amp.SDK/stdlib/task"
)

// App is a helper for implementing AppInstance.
// An amp.App implementation embeds this into their app instance struct, instantly providing a skeleton of amp.AppInstance interface.
type App[AppT amp.AppInstance] struct {
	amp.AppContext
	Instance AppT
}

// Cell is how std makes calls against a cell
type Cell[AppT amp.AppInstance] interface {
	Root() *CellNode[AppT]

	// Tells this cell it has been pinned and should synchronously update itself accordingly.
	PinInto(dst *Pin[AppT]) error

	// MarshalAttrs is called after PinInto to serialize the cell's pinned attributes.
	MarshalAttrs(w CellWriter)
}

// CellNode is a helper for implementing the Cell interface.
type CellNode[AppT amp.AppInstance] struct {
	ID tag.ID
}

// Wraps the pinned state of a cell -- implements amp.Pin
type Pin[AppT amp.AppInstance] struct {
	Op   amp.Requester // originating request
	Cell Cell[AppT]    // pinned cell
	App  AppT          // parent app instance
	Sync amp.StateSync // Op.Request().StateSync

	children map[tag.ID]Cell[AppT] // child cells
	ctx      task.Context          // task context for this pin
}

type CellWriter interface {

	// Pushes a tx operation attribute to the cell's pinned state.
	Push(op *amp.TxOp, value tag.Value)

	// Convenience methods for pushing string and generic attributes bound to an item ID.
	PushTextWithID(attrID tag.ID, itemID tag.ID, value string)
	PushItemWithID(attrID tag.ID, itemID tag.ID, value tag.Value)

	// Convenience methods for pushing string and generic tag.Value attributes bound to std.Item000 (aka tag.ID{})
	// Push*WithID(), if the value is nil, the attribute item is skipped.
	PushText(attrID tag.ID, value string)
	PushItem(attrID tag.ID, value tag.Value)
}

const (
	FactoryURL = "file://_resources_/"
)
