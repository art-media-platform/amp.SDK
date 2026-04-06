// Package std provides Go support classes for building amp apps.
//
// Most amp apps embed AppModule and use Pin to manage bidirectional state flow.
// Send-only apps (e.g. serving a file listing) use the Item/PinAndServe pattern.
// Interactive apps (e.g. planet viewers, editors) use Pin.Bind/MergeIncoming with AttrBinding.
package std

import (
	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"github.com/art-media-platform/amp.SDK/stdlib/task"
	"google.golang.org/protobuf/proto"
)

// AppModule is a helper for implementing AppInstance.
// An amp.AppModule implementation embeds this into their app instance struct, instantly providing a skeleton of amp.AppInstance interface.
type AppModule[AppT amp.AppInstance] struct {
	amp.AppContext
	Instance AppT
}

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

// Pin manages bidirectional state flow for an amp app request.
//
// For send-only apps (Item pattern):
//   - PinAndServe populates items via PinInto, serializes via MarshalAttrs, pushes to client.
//
// For interactive apps (binding pattern):
//   - Bind registers NodeResponders (typically AttrBinding instances).
//   - MergeIncoming dispatches incoming TxMsg ops to bound responders.
//   - Responders fire typed callbacks per item, enabling reactive state management.
type Pin[AppT amp.AppInstance] struct {
	Request *amp.Request // originating request
	Item    Item[AppT]   // pinned item (nil for binding-only pins)
	App     AppT         // parent app instance
	Sync    amp.PinMode  // Op.Request().PinMode

	fatal      error                  // fatal error, if any
	children   map[tag.UID]Item[AppT] // child items (Item pattern)
	ctx        task.Context           // task context for this pin
	responders []amp.NodeResponder    // bound responders (binding pattern)
}

// Bind registers a NodeResponder on this pin.
// When MergeIncoming is called, matching ops are dispatched to this responder.
// Typically used with amp.AttrBinding[V] instances.
func (pin *Pin[AppT]) Bind(resp amp.NodeResponder) {
	pin.responders = append(pin.responders, resp)
}

// MergeIncoming processes an incoming TxMsg by dispatching ops to bound responders.
// Supports multi-pass iteration: if a responder adds new responders during its callback,
// additional passes ensure the new responders also process the tx.
func (pin *Pin[AppT]) MergeIncoming(tx *amp.TxMsg) {
	revision := tag.NowID()

	// Collect unique nodeIDs from the tx ops.
	nodeIDs := make([]tag.UID, 0, 4)
	seen := make(map[tag.UID]bool, 4)
	for _, op := range tx.Ops {
		if !seen[op.Addr.NodeID] {
			seen[op.Addr.NodeID] = true
			nodeIDs = append(nodeIDs, op.Addr.NodeID)
		}
	}

	// Multi-pass: responders added during callbacks trigger additional passes.
	for range 100 {
		prevCount := len(pin.responders)

		for _, nodeID := range nodeIDs {
			update := amp.NodeUpdate{
				Tx:       tx,
				NodeID:   nodeID,
				Revision: revision,
			}
			for _, resp := range pin.responders {
				if resp.Revision() != revision {
					resp.OnNodeUpdate(update)
				}
			}
		}

		if len(pin.responders) == prevCount {
			break
		}
	}
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
