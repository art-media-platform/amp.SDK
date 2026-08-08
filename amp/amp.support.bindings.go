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
	Tx      *TxMsg // carrying tx; borrowed for the callback only (read Tx.TxID(), never retain)
}

// ItemMerger folds an arriving op's decoded value into an item's cached value —
// the per-attr override of FoldBinding's whole-value LWW, for attrs whose
// record custody is per-field (MemberEpoch: NewMemberEpochMerger).
//
// MergeItem sees every admitted non-delete arrival, stale EditIDs included;
// idempotence under re-presentation is the merger's contract (strict per-field
// clocks).  prev is the current cached value (hasPrev false on first sight);
// the returned merged value replaces the cache when changed.  arrival.Value is
// owned by the arrival (freshly unmarshaled) but is also handed to OnItem
// consumers — a merger that retains sub-messages must clone them.  DropItem
// clears any per-item merge state when the item is deleted.
type ItemMerger[V proto.Message] interface {
	MergeItem(arrival AttrItem[V], prev V, hasPrev bool) (merged V, changed bool)
	DropItem(itemID tag.UID)
}

// FoldBinding wires a specific attr (and optional item filter) to typed callbacks and caches item state.
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
type FoldBinding[V proto.Message] struct {
	Attr tag.Name // attr to match
	Item tag.Name // item to match (wildcard matches all)

	// OnAdmit, if set, is a pre-commit veto invoked for each matching op AFTER the
	// (node, attr, item) filter but BEFORE the CRDT edit-ordering check and cache:
	// return false to drop the op entirely (no edit recorded, no cache, no OnItem),
	// so a vetoed op cannot advance the item's high-water EditID.  Receives the op
	// address and the carrying tx (e.g. for tx.TxID() recency / skew / cap policy).
	OnAdmit func(addr tag.Address, tx *TxMsg) bool

	// OnItem fires for each matching op that passes the admit check and CRDT ordering (if non-nil).
	// With a Merger set it fires for EVERY admitted non-delete arrival (stale EditIDs included)
	// and receives the ARRIVING record verbatim — never the merged cache value — so consumers
	// that act on what a record carries (e.g. WrappedKeys extraction) see each delivery.
	OnItem func(item AttrItem[V])

	// OnSync fires once after all ops in a NodeUpdate have been processed  (if non-nil)
	OnSync func()

	// Merger, if set, replaces the cache's whole-value LWW rule with a per-field
	// CRDT fold (see ItemMerger): every non-delete arrival is offered to the
	// merger — a stale-ordered record can still own a field — and the cache
	// (GetItem / EnumItems / FirstItem) serves the MERGED value.  Deletes keep
	// LWW ordering.  OnItem behavior: see above.
	Merger ItemMerger[V]

	revision tag.UID                  // most recently witnessed NodeUpdate
	nodeID   tag.UID                  // bound node ID
	msgType  protoreflect.MessageType // proto factory for V (resolved once at construction)
	edits    map[tag.UID]tag.UID      // ItemID -> EditID (CRDT ordering)
	items    map[tag.UID]V            // ItemID -> cached value (live items only; deleted items removed)
}

// NewFoldBinding creates a binding that matches ALL item IDs for the given attr.
func NewFoldBinding[V proto.Message](attrID tag.Name) *FoldBinding[V] {
	var zero V
	return &FoldBinding[V]{
		Attr:    attrID,
		Item:    tag.Wildcard(),
		msgType: zero.ProtoReflect().Type(),
		edits:   make(map[tag.UID]tag.UID, 64),
		items:   make(map[tag.UID]V, 64),
	}
}

// NewFoldItemBinding creates a binding for a specific attr and item ID.
func NewFoldItemBinding[V proto.Message](attrID, itemID tag.Name) *FoldBinding[V] {
	capHint := 1
	if itemID.IsWildcard() {
		capHint = 64
	}
	var zero V
	return &FoldBinding[V]{
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
func (b *FoldBinding[V]) Revision() tag.UID { return b.revision }

// NodeID returns the bound node ID (set by Bind or auto-detected from first update).
func (b *FoldBinding[V]) NodeID() tag.UID { return b.nodeID }

// ════════════════════════════════════════════════════════
// Read: accumulated item state
// ════════════════════════════════════════════════════════

// ItemCount returns the number of live (non-deleted) items.
func (b *FoldBinding[V]) ItemCount() int {
	return len(b.items)
}

// HasItem returns true if the binding has a live value for the given item.
func (b *FoldBinding[V]) HasItem(itemID tag.UID) bool {
	_, ok := b.items[itemID]
	return ok
}

// GetItem returns the most recent value for an item, or (zero, false) if absent or deleted.
func (b *FoldBinding[V]) GetItem(itemID tag.UID) (V, bool) {
	val, ok := b.items[itemID]
	return val, ok
}

// FirstItem returns any single live item — useful for single-value (non-wildcard) bindings.
// Mirrors C# ItemNode.LoadAttrItem<V>.
func (b *FoldBinding[V]) FirstItem() (tag.UID, V, bool) {
	for id, val := range b.items {
		return id, val, true
	}
	var zero V
	return tag.UID{}, zero, false
}

// EnumItems iterates all live items.  Return false from fn to stop early.
// Iteration order is not guaranteed.
func (b *FoldBinding[V]) EnumItems(fn func(itemID tag.UID, value V) bool) {
	for id, val := range b.items {
		if !fn(id, val) {
			return
		}
	}
}

// EnumItemIDs iterates the IDs of all live items.
func (b *FoldBinding[V]) EnumItemIDs(fn func(itemID tag.UID) bool) {
	for id := range b.items {
		if !fn(id) {
			return
		}
	}
}

// ItemAddress returns the full address (including EditID) for a tracked item.
// Returns false if the item has never been seen.
func (b *FoldBinding[V]) ItemAddress(itemID tag.UID) (addr tag.Address, ok bool) {
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
func (b *FoldBinding[V]) Clear() {
	clear(b.edits)
	clear(b.items)
	b.revision = tag.UID{}
}

// ════════════════════════════════════════════════════════
// Write
// ════════════════════════════════════════════════════════

// Bind explicitly sets the node ID for this binding.
// Required before Upsert if the binding hasn't yet received an incoming update.
func (b *FoldBinding[V]) Bind(nodeID tag.UID) {
	if b.nodeID.IsSet() && b.nodeID != nodeID {
		panic("FoldBinding: Bind called with different nodeID")
	}
	b.nodeID = nodeID
}

// UpsertItem writes an upsert op into tx using this binding's node and attr.
func (b *FoldBinding[V]) UpsertItem(tx *TxMsg, itemID tag.UID, value V) error {
	if b.nodeID.IsNil() {
		panic("FoldBinding: UpsertItem called before Bind or first update")
	}
	return tx.Upsert(b.nodeID, b.Attr.ID, itemID, value)
}

// DeleteItem appends a delete op for a known item.  Returns false if the item is unknown.
func (b *FoldBinding[V]) DeleteItem(tx *TxMsg, itemID tag.UID) bool {
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
func (b *FoldBinding[V]) OnNodeUpdate(update NodeUpdate) {
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

		// Pre-commit veto: runs before the edit-ordering check so a rejected op
		// never advances the item's high-water EditID (no cache poisoning).
		if b.OnAdmit != nil && !b.OnAdmit(op.Addr, tx) {
			continue
		}

		// CRDT ordering: whole-value LWW by EditID.  A Merger-equipped binding
		// folds every non-delete arrival instead (a stale-ordered record can
		// still own a field — per-field custody); deletes keep LWW.
		prevEdit, hasEdit := b.edits[op.Addr.ItemID]
		isDelete := (op.Flags & TxOpFlags_Delete) != 0
		if (b.Merger == nil || isDelete) && hasEdit && prevEdit.CompareTo(op.Addr.EditID) >= 0 {
			continue
		}
		if !hasEdit || prevEdit.CompareTo(op.Addr.EditID) < 0 {
			b.edits[op.Addr.ItemID] = op.Addr.EditID
		}

		item := AttrItem[V]{
			Addr:  op.Addr,
			Tx:    tx,
			Value: b.msgType.New().Interface().(V),
		}

		if isDelete {
			item.Deleted = true
			delete(b.items, op.Addr.ItemID)
			if b.Merger != nil {
				b.Merger.DropItem(op.Addr.ItemID)
			}
			if !hasEdit {
				continue // ignore deletes of items we've never seen
			}
		} else {
			if err := tx.UnmarshalOpValue(idx, item.Value); err != nil {
				continue
			}
			if b.Merger != nil {
				prev, hasPrev := b.items[op.Addr.ItemID]
				if merged, changed := b.Merger.MergeItem(item, prev, hasPrev); changed {
					b.items[op.Addr.ItemID] = merged
				}
			} else {
				b.items[op.Addr.ItemID] = item.Value
			}
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
			Tx:    tx,
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
