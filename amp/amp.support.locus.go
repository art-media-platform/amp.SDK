package amp

import (
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Locus defaults — mirrored from amp/std to avoid import cycle (amp ↔ amp/std).
const (
	locusDefaultMax  = 37 // LocusHexCells: 3 hex rings
	locusDefaultHint = 7  // LocusTier1: center + ring 1
)

// locusCell is one occupied cell in a LocusBinding.
type locusCell[V proto.Message] struct {
	cell   int     // 0..63
	editID tag.UID // CRDT ordering
	value  V       // cached proto value
}

// LocusBinding wires a contiguous locus span (base AttrID | cellIndex) to typed callbacks.
// V is the concrete proto.Message type for each cell's value.
//
// A locus covers up to 64 cells addressed by the low 6 bits of the AttrID UID.
// Only occupied cells are stored (sparse attachment slice).  With ≤37 hex cells,
// linear scan beats map overhead and is cache-friendly.
type LocusBinding[V proto.Message] struct {
	Base tag.Name // base attr (cell 0); low 6 bits of ID must be 0
	Max  int      // max cell count (default LocusHexCells = 37)

	OnCell func(cell int, item AttrItem[V]) // per-cell update callback
	OnSync func()                           // batch complete callback

	revision tag.UID
	nodeID   tag.UID
	msgType  protoreflect.MessageType
	attached []locusCell[V] // sparse list of occupied cells
}

// NewLocusBinding creates a locus binding for the given base attr, matching up to LocusHexCells cells.
func NewLocusBinding[V proto.Message](base tag.Name) *LocusBinding[V] {
	var zero V
	return &LocusBinding[V]{
		Base:     base,
		Max:      locusDefaultMax,
		msgType:  zero.ProtoReflect().Type(),
		attached: make([]locusCell[V], 0, locusDefaultHint),
	}
}

// ════════════════════════════════════════════════════════
// Identity
// ════════════════════════════════════════════════════════

// Revision implements NodeResponder.
func (loc *LocusBinding[V]) Revision() tag.UID { return loc.revision }

// NodeID returns the bound node ID.
func (loc *LocusBinding[V]) NodeID() tag.UID { return loc.nodeID }

// Bind explicitly sets the node ID for this binding.
func (loc *LocusBinding[V]) Bind(nodeID tag.UID) {
	if loc.nodeID.IsSet() && loc.nodeID != nodeID {
		panic("LocusBinding: Bind called with different nodeID")
	}
	loc.nodeID = nodeID
}

// ════════════════════════════════════════════════════════
// Read: accumulated cell state
// ════════════════════════════════════════════════════════

// LiveCount returns the number of occupied cells.
func (loc *LocusBinding[V]) LiveCount() int { return len(loc.attached) }

// GetCell returns the cached value for a cell, or (zero, false) if unoccupied.
func (loc *LocusBinding[V]) GetCell(cell int) (V, bool) {
	for i := range loc.attached {
		if loc.attached[i].cell == cell {
			return loc.attached[i].value, true
		}
	}
	var zero V
	return zero, false
}

// EnumCells iterates occupied cells.  Return false from fn to stop early.
func (loc *LocusBinding[V]) EnumCells(fn func(cell int, value V) bool) {
	for idx := range loc.attached {
		if !fn(loc.attached[idx].cell, loc.attached[idx].value) {
			return
		}
	}
}

// ════════════════════════════════════════════════════════
// Write
// ════════════════════════════════════════════════════════

// UpsertCell writes an upsert op for the given cell into tx.
func (loc *LocusBinding[V]) UpsertCell(tx *TxMsg, cell int, value V) error {
	if loc.nodeID.IsNil() {
		panic("LocusBinding: UpsertCell called before Bind or first update")
	}
	attrID := loc.Base.ID.WithCell(cell)
	return tx.Upsert(loc.nodeID, attrID, tag.UID{}, value)
}

// DeleteCell appends a delete op for the given cell.  Returns false if the cell is unoccupied.
func (loc *LocusBinding[V]) DeleteCell(tx *TxMsg, cell int) bool {
	att := loc.findCell(cell)
	if att == nil {
		return false
	}
	attrID := loc.Base.ID.WithCell(cell)
	_ = tx.Delete(tag.ElementID{
		NodeID: loc.nodeID,
		AttrID: attrID,
	}, nil)
	return true
}

// ════════════════════════════════════════════════════════
// Incoming: NodeResponder implementation
// ════════════════════════════════════════════════════════

// OnNodeUpdate filters incoming ops for this locus span, updates cached cell state,
// and fires OnCell for each matching op.
func (loc *LocusBinding[V]) OnNodeUpdate(update NodeUpdate) {
	loc.revision = update.Revision

	if loc.nodeID != update.NodeID {
		if loc.nodeID.IsNil() {
			loc.nodeID = update.NodeID
		} else {
			return
		}
	}

	baseID := loc.Base.ID
	for idx, op := range update.Tx.Ops {
		if op.Addr.NodeID != loc.nodeID {
			continue
		}
		if !baseID.LocusMatch(op.Addr.AttrID) {
			continue
		}
		cell := op.Addr.AttrID.LocusCell()
		if cell >= loc.Max {
			continue
		}

		att := loc.findCell(cell)

		// CRDT: reject stale edits
		if att != nil && att.editID.CompareTo(op.Addr.EditID) >= 0 {
			continue
		}

		item := AttrItem[V]{
			Addr:  op.Addr,
			Value: loc.msgType.New().Interface().(V),
		}

		if (op.Flags & TxOpFlags_Delete) != 0 {
			item.Deleted = true
			loc.detachCell(cell)
		} else {
			if err := update.Tx.UnmarshalOpValue(idx, item.Value); err != nil {
				continue
			}
			if att == nil {
				loc.attached = append(loc.attached, locusCell[V]{cell: cell, editID: op.Addr.EditID, value: item.Value})
			} else {
				att.editID = op.Addr.EditID
				att.value = item.Value
			}
		}
		if loc.OnCell != nil {
			loc.OnCell(cell, item)
		}
	}

	if loc.OnSync != nil {
		loc.OnSync()
	}
}

// findCell returns a pointer to the attachment for cell, or nil.
func (loc *LocusBinding[V]) findCell(cell int) *locusCell[V] {
	for idx := range loc.attached {
		if loc.attached[idx].cell == cell {
			return &loc.attached[idx]
		}
	}
	return nil
}

// detachCell removes a cell by swap-delete (O(1) removal, order doesn't matter).
func (loc *LocusBinding[V]) detachCell(cell int) {
	for idx := range loc.attached {
		if loc.attached[idx].cell == cell {
			loc.attached[idx] = loc.attached[len(loc.attached)-1]
			loc.attached = loc.attached[:len(loc.attached)-1]
			return
		}
	}
}

// Clear resets all accumulated state.
func (loc *LocusBinding[V]) Clear() {
	loc.attached = loc.attached[:0]
	loc.revision = tag.UID{}
}
