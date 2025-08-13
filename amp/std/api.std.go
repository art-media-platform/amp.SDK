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

// Item is how std makes calls against a item
type Item[AppT amp.AppInstance] interface {
	Root() *ItemNode[AppT]

	// Tells this item it has been pinned and should synchronously update itself accordingly.
	PinInto(dst *Pin[AppT]) error

	// MarshalAttrs is called after PinInto to serialize the item's pinned attributes.
	MarshalAttrs(w ItemWriter)
}

// ItemNode is a helper for implementing the Item interface.
type ItemNode[AppT amp.AppInstance] struct {
	ID tag.UID
}

// Wraps the pinned state of a item -- implements amp.Pin
type Pin[AppT amp.AppInstance] struct {
	Request *amp.Request // originating request
	Item    Item[AppT]   // pinned item
	App     AppT         // parent app instance
	Sync    amp.PinMode  // Op.Request().PinMode

	fatal    error                  // fatal error, if any
	children map[tag.UID]Item[AppT] // child items
	ctx      task.Context           // task context for this pin
}

type ItemWriter interface {

	// Convenience methods for pushing string and generic attributes bound to an item ID.
	PutTextAt(attrID, itemID tag.UID, value string)
	PutItemAt(attrID, itemID tag.UID, value amp.Value)

	// Convenience methods for pushing an attribute value at item 0,0,0.
	// Push*WithID(), if the value is nil, the attribute item is skipped.
	PutText(attrID tag.UID, value string)
	PutItem(attrID tag.UID, value amp.Value)
}

const (
	FactoryURL = "file://_resources_/"
)
