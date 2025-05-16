// A collection of types and functions that form a support template when building an amp.AppModule.
// Most amp.Apps build upon this template but a specialized app may opt to build their own foundation upon the amp api.
package std

import (
	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"github.com/art-media-platform/amp.SDK/stdlib/task"
)

// AppModule is a helper for implementing AppInstance.
// An amp.AppModule implementation embeds this into their app instance struct, instantly providing a skeleton of amp.AppInstance interface.
type AppModule[AppT amp.AppInstance] struct {
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
	ID tag.U3D
}

// Wraps the pinned state of a cell -- implements amp.Pin
type Pin[AppT amp.AppInstance] struct {
	Request *amp.Request // originating request
	Cell    Cell[AppT]   // pinned cell
	App     AppT         // parent app instance
	Sync    amp.PinMode  // Op.Request().PinMode

	fatal    error                  // fatal error, if any
	children map[tag.U3D]Cell[AppT] // child cells
	ctx      task.Context           // task context for this pin
}

type CellWriter interface {

	// Pushes a tx operation attribute to the cell's pinned state.
	Push(op *amp.TxOp, value amp.Value)

	// Convenience methods for pushing string and generic attributes bound to an item ID.
	PushTextWithID(attrID tag.UID, itemID tag.U3D, value string)
	PushItemWithID(attrID tag.UID, itemID tag.U3D, value amp.Value)

	// Convenience methods for pushing an attribute value at item 0,0,0.
	// Push*WithID(), if the value is nil, the attribute item is skipped.
	PushText(attrID tag.UID, value string)
	PushItem(attrID tag.UID, value amp.Value)
}

const (
	FactoryURL = "file://_resources_/"
)
