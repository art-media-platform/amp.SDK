package amp

import (
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// OpRange is a simplified address prefix filter used by bindings.
type OpRange struct {
	Addr tag.Address
}

// NodeUpdate encapsulates a new TxMsg that updates a node and an optional range.
type NodeUpdate struct {
	NodeID   tag.UID
	Revision tag.UID
	SubRange OpRange
	Tx       *TxMsg
}

// NodeResponder receives updates when a node's ops change.
type NodeResponder interface {
	Revision() tag.UID
	OnNodeUpdate(update NodeUpdate)
}

// AttrItem is the Go representation of a single attr item update.
// If Deleted is true, Value is a zero-value instance of V.
type AttrItem[V proto.Message] struct {
	Addr    tag.Address
	Value   V
	Deleted bool
}

// AttrBinding wires a specific attr (and optional item filter) to typed callbacks and caches item state.
// V is the concrete proto.Message type expected for this attr (must be a pointer type like *amp.Tag).
//
// Reading:
//
//	OnItem fires for each matching item in incoming TxMsgs.
//	GetItem, HasItem, ItemCount, EnumItems, FirstItem provide access to the accumulated item state.
//
// Writing:
//
//	Bind + UpsertItem / DeleteItem let you author ops through this binding's node/attr context.
type AttrBinding[V proto.Message] struct {
	Attr tag.Name // attr to match
	Item tag.Name // item to match (wildcard matches all)

	OnItem func(item AttrItem[V]) // per-item update callback (optional)
	OnSync func()                 // called after all items in an update are dispatched (optional)

	revision tag.UID                  // most recently witnessed NodeUpdate
	nodeID   tag.UID                  // bound node ID
	msgType  protoreflect.MessageType // proto factory for V (resolved once at construction)
	edits    map[tag.UID]tag.UID      // ItemID -> EditID (CRDT ordering)
	items    map[tag.UID]V            // ItemID -> cached value (live items only; deleted items removed)
}

// NewAttrBinding creates a binding that matches ALL item IDs for the given attr.
func NewAttrBinding[V proto.Message](attrID tag.Name) *AttrBinding[V] {
	var zero V
	return &AttrBinding[V]{
		Attr:    attrID,
		Item:    tag.Wildcard(),
		msgType: zero.ProtoReflect().Type(),
		edits:   make(map[tag.UID]tag.UID, 64),
		items:   make(map[tag.UID]V, 64),
	}
}

// NewAttrItemBinding creates a binding for a specific attr and item ID.
func NewAttrItemBinding[V proto.Message](attrID, itemID tag.Name) *AttrBinding[V] {
	capHint := 1
	if itemID.IsWildcard() {
		capHint = 64
	}
	var zero V
	return &AttrBinding[V]{
		Attr:    attrID,
		Item:    itemID,
		msgType: zero.ProtoReflect().Type(),
		edits:   make(map[tag.UID]tag.UID, capHint),
		items:   make(map[tag.UID]V, capHint),
	}
}

// ════════════════════════════════════════════════════════
// Identity
// ════════════════════════════════════════════════════════

// Revision implements NodeResponder.
func (b *AttrBinding[V]) Revision() tag.UID { return b.revision }

// NodeID returns the bound node ID (set by Bind or auto-detected from first update).
func (b *AttrBinding[V]) NodeID() tag.UID { return b.nodeID }

// ════════════════════════════════════════════════════════
// Read: accumulated item state
// ════════════════════════════════════════════════════════

// ItemCount returns the number of live (non-deleted) items.
func (b *AttrBinding[V]) ItemCount() int {
	return len(b.items)
}

// HasItem returns true if the binding has a live value for the given item.
func (b *AttrBinding[V]) HasItem(itemID tag.UID) bool {
	_, ok := b.items[itemID]
	return ok
}

// GetItem returns the most recent value for an item, or (zero, false) if absent or deleted.
func (b *AttrBinding[V]) GetItem(itemID tag.UID) (V, bool) {
	val, ok := b.items[itemID]
	return val, ok
}

// FirstItem returns any single live item — useful for single-value (non-wildcard) bindings.
// Mirrors C# ItemNode.LoadAttrItem<V>.
func (b *AttrBinding[V]) FirstItem() (tag.UID, V, bool) {
	for id, val := range b.items {
		return id, val, true
	}
	var zero V
	return tag.UID{}, zero, false
}

// EnumItems iterates all live items.  Return false from fn to stop early.
// Iteration order is not guaranteed.
func (b *AttrBinding[V]) EnumItems(fn func(itemID tag.UID, value V) bool) {
	for id, val := range b.items {
		if !fn(id, val) {
			return
		}
	}
}

// EnumItemIDs iterates the IDs of all live items.
func (b *AttrBinding[V]) EnumItemIDs(fn func(itemID tag.UID) bool) {
	for id := range b.items {
		if !fn(id) {
			return
		}
	}
}

// ItemAddress returns the full address (including EditID) for a tracked item.
// Returns false if the item has never been seen.
func (b *AttrBinding[V]) ItemAddress(itemID tag.UID) (addr tag.Address, ok bool) {
	editID, ok := b.edits[itemID]
	if ok {
		addr.NodeID = b.nodeID
		addr.AttrID = b.Attr.ID
		addr.ItemID = itemID
		addr.EditID = editID
	}
	return addr, ok
}

// Clear resets all accumulated state (edits, cached values, revision).
// The binding remains usable — subsequent updates repopulate it.
func (b *AttrBinding[V]) Clear() {
	clear(b.edits)
	clear(b.items)
	b.revision = tag.UID{}
}

// ════════════════════════════════════════════════════════
// Write
// ════════════════════════════════════════════════════════

// Bind explicitly sets the node ID for this binding.
// Required before Upsert if the binding hasn't yet received an incoming update.
func (b *AttrBinding[V]) Bind(nodeID tag.UID) {
	if b.nodeID.IsSet() && b.nodeID != nodeID {
		panic("AttrBinding: Bind called with different nodeID")
	}
	b.nodeID = nodeID
}

// UpsertItem writes an upsert op into tx using this binding's node and attr.
func (b *AttrBinding[V]) UpsertItem(tx *TxMsg, itemID tag.UID, value V) error {
	if b.nodeID.IsNil() {
		panic("AttrBinding: UpsertItem called before Bind or first update")
	}
	return tx.Upsert(b.nodeID, b.Attr.ID, itemID, value)
}

// DeleteItem appends a delete op for a known item.  Returns false if the item is unknown.
func (b *AttrBinding[V]) DeleteItem(tx *TxMsg, itemID tag.UID) bool {
	addr, ok := b.ItemAddress(itemID)
	if !ok {
		return false
	}
	_ = tx.Delete(addr.ElementID, nil)
	return true
}

// ════════════════════════════════════════════════════════
// Incoming: NodeResponder implementation
// ════════════════════════════════════════════════════════

// OnNodeUpdate filters incoming ops for this attr/item, updates the cached item state,
// and fires OnItem for each matching op.
func (b *AttrBinding[V]) OnNodeUpdate(update NodeUpdate) {
	b.revision = update.Revision

	// Auto-bind to the first node we see; skip updates for other nodes.
	if b.nodeID != update.NodeID {
		if b.nodeID.IsNil() {
			b.nodeID = update.NodeID
		} else {
			return
		}
	}

	tx := update.Tx
	nodeID := b.nodeID
	attrID := b.Attr.ID
	matchAllItems := b.Item.IsWildcard()
	filterItemID := b.Item.ID

	for idx, op := range tx.Ops {
		if op.Addr.NodeID != nodeID || op.Addr.AttrID != attrID {
			continue
		}
		if !matchAllItems && op.Addr.ItemID != filterItemID {
			continue
		}

		// CRDT: reject edits older than what we already have.
		prevEdit, hasEdit := b.edits[op.Addr.ItemID]
		if hasEdit && prevEdit.CompareTo(op.Addr.EditID) >= 0 {
			continue
		}
		b.edits[op.Addr.ItemID] = op.Addr.EditID

		item := AttrItem[V]{
			Addr:  op.Addr,
			Value: b.msgType.New().Interface().(V),
		}

		if (op.Flags & TxOpFlags_Delete) != 0 {
			item.Deleted = true
			delete(b.items, op.Addr.ItemID)
			if !hasEdit {
				continue // ignore deletes of items we've never seen
			}
		} else {
			if err := tx.UnmarshalOpValue(idx, item.Value); err != nil {
				continue
			}
			b.items[op.Addr.ItemID] = item.Value
		}

		if b.OnItem != nil {
			b.OnItem(item)
		}
	}

	if b.OnSync != nil {
		b.OnSync()
	}
}

// ════════════════════════════════════════════════════════
// TxMsg query helpers
// ════════════════════════════════════════════════════════

// HasAttr returns true if tx has any ops for the given node and attr.
func HasAttr(tx *TxMsg, nodeID, attrID tag.UID) bool {
	for _, op := range tx.Ops {
		if op.Addr.NodeID == nodeID && op.Addr.AttrID == attrID {
			return true
		}
	}
	return false
}

// EnumNodeIDs returns distinct node IDs in the tx, in order of first appearance.
func EnumNodeIDs(tx *TxMsg) []tag.UID {
	seen := make(map[tag.UID]bool, 4)
	out := make([]tag.UID, 0, 4)
	for _, op := range tx.Ops {
		if !seen[op.Addr.NodeID] {
			seen[op.Addr.NodeID] = true
			out = append(out, op.Addr.NodeID)
		}
	}
	return out
}

// ExtractItems unmarshals all ops matching (nodeID, attrID) from a TxMsg into typed AttrItems.
// Useful for one-off reads without setting up a persistent binding.
func ExtractItems[V proto.Message](tx *TxMsg, nodeID, attrID tag.UID) []AttrItem[V] {
	var zero V
	msgType := zero.ProtoReflect().Type()
	var out []AttrItem[V]

	for idx, op := range tx.Ops {
		if op.Addr.NodeID != nodeID || op.Addr.AttrID != attrID {
			continue
		}
		item := AttrItem[V]{
			Addr:  op.Addr,
			Value: msgType.New().Interface().(V),
		}
		if (op.Flags & TxOpFlags_Delete) != 0 {
			item.Deleted = true
		} else {
			if err := tx.UnmarshalOpValue(idx, item.Value); err != nil {
				continue
			}
		}
		out = append(out, item)
	}
	return out
}
