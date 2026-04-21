package tag

import (
	"fmt"
	"math"
	"sync"
)

// Locus constants — mirrored from amp/std to avoid import cycle (tag ← std ← amp ← tag).
// Source of truth: amp.std.consts.sdl
const (
	locusMask     = uint64(0x3F) // low 6-bit extraction mask
	locusSpan     = 64           // total cells per locus (1 << 6)
	locusHexCells = 37           // 3 hex rings: 1+6+12+18
)

// LocusBase zeros the low 6 bits of UID[1], yielding the locus base AttrID.
// A locus base + cell index spans 64 contiguous UIDs without additional hashing.
func (uid UID) LocusBase() UID {
	return UID{uid[0], uid[1] &^ locusMask}
}

// LocusCell returns the cell index (0..63) from the low 6 bits of UID[1].
func (uid UID) LocusCell() int {
	return int(uid[1] & locusMask)
}

// WithCell returns base | cellIndex.  The receiver must be a locus base (low 6 bits = 0).
func (uid UID) WithCell(cell int) UID {
	return UID{uid[0], uid[1] | uint64(cell)&locusMask}
}

// LocusMatch reports whether uid falls within the locus span of base.
func (base UID) LocusMatch(uid UID) bool {
	return uid[0] == base[0] && (uid[1]&^locusMask) == (base[1]&^locusMask)
}

// ════════════════════════════════════════════════════════
// Hex Spiral Layout
// ════════════════════════════════════════════════════════

// HexOffset holds an axial (q, r) coordinate for one hex cell.
type HexOffset struct{ Q, R int8 }

// LocusHexSpiral maps cell ordinals 0..36 to axial hex coordinates.
// Center = 0, ring 1 = 1-6 (CCW from +Q), ring 2 = 7-18, ring 3 = 19-36.
var LocusHexSpiral = [locusHexCells]HexOffset{
	{0, 0}, // center
	// ring 1
	{1, 0}, {0, 1}, {-1, 1}, {-1, 0}, {0, -1}, {1, -1},
	// ring 2
	{2, 0}, {1, 1}, {0, 2}, {-1, 2}, {-2, 2}, {-2, 1},
	{-2, 0}, {-1, -1}, {0, -2}, {1, -2}, {2, -2}, {2, -1},
	// ring 3
	{3, 0}, {2, 1}, {1, 2}, {0, 3}, {-1, 3}, {-2, 3},
	{-3, 3}, {-3, 2}, {-3, 1}, {-3, 0}, {-2, -1}, {-1, -2},
	{0, -3}, {1, -3}, {2, -3}, {3, -3}, {3, -2}, {3, -1},
}

const hexSqrt3_2 = 0.86602540378 // √3 / 2

// LocusLayout maps a cell index and spacing to a world-space position and facing angle (radians).
type LocusLayout func(cell int, spacing float64) (px, pz, angle float64)

// LayoutHexSpiral places cells in a pointy-top hex spiral pattern.
var LayoutHexSpiral LocusLayout = locusHexSpiralPos

// LayoutLinear places cells sequentially along the X axis.
var LayoutLinear LocusLayout = locusLinearPos

// LayoutRadialRing places cells on concentric circles facing center.
var LayoutRadialRing LocusLayout = locusRadialPos

// LayoutTorus places cells on an 8×8 flat torus; clients wrap on both axes.
var LayoutTorus LocusLayout = locusTorusPos

func locusHexSpiralPos(cell int, spacing float64) (px, pz, angle float64) {
	if cell < 0 || cell >= locusHexCells {
		return 0, 0, 0
	}
	hex := LocusHexSpiral[cell]
	qf := float64(hex.Q)
	rf := float64(hex.R)
	px = spacing * (qf + rf*0.5)
	pz = spacing * rf * hexSqrt3_2
	return px, pz, 0
}

func locusLinearPos(cell int, spacing float64) (px, pz, angle float64) {
	return float64(cell) * spacing, 0, 0
}

// locusTorusPos maps cell 0..63 onto an 8×8 grid.  Consumers interpret (px, pz)
// as offsets along the two periodic axes and wrap with modulo 8*spacing.
func locusTorusPos(cell int, spacing float64) (px, pz, angle float64) {
	if cell < 0 || cell >= locusSpan {
		return 0, 0, 0
	}
	return float64(cell%8) * spacing, float64(cell/8) * spacing, 0
}

func locusRadialPos(cell int, spacing float64) (px, pz, angle float64) {
	if cell <= 0 {
		return 0, 0, 0
	}
	ring, indexInRing := cellToRing(cell)
	cellsInRing := 6 * ring
	theta := 2 * math.Pi * float64(indexInRing) / float64(cellsInRing)
	radius := float64(ring) * spacing
	return radius * math.Cos(theta), radius * math.Sin(theta), theta
}

// cellToRing returns the ring number (1-based) and index within that ring for a hex cell ordinal > 0.
// Ring 1: cells 1-6, ring 2: cells 7-18, ring 3: cells 19-36.
func cellToRing(cell int) (ring, indexInRing int) {
	ring = 1
	start := 1
	for {
		count := 6 * ring
		if cell < start+count {
			return ring, cell - start
		}
		start += count
		ring++
	}
}

// ════════════════════════════════════════════════════════
// LocusKit — pluggable spatial placement conventions
// ════════════════════════════════════════════════════════

// LocusKitID identifies a pluggable spatial convention for arranging locus cells.
// Mirrors the pattern of CryptoKitID / HashKitID in the safe package.
type LocusKitID int32

const (
	LocusKit_Unspecified LocusKitID = 0
	LocusKit_HexSpiral   LocusKitID = 1 // 37-cell 3-ring pointy-top hex layout (default)
	LocusKit_Linear      LocusKitID = 2 // cells laid out along a single axis
	LocusKit_RadialRing  LocusKitID = 3 // concentric circles facing center
	LocusKit_S2          LocusKitID = 4 // continuous sphere via Google S2 (registered by amp.planet)
	LocusKit_Torus       LocusKitID = 5 // 8×8 flat torus with wraparound on both axes
	// Reserved for future:
	//   LocusKit_Cube  = 6   // 3D voxel grid
)

// LocusKit describes a pluggable spatial placement convention: a fixed cell capacity
// and a layout function that maps cell ordinals to world-space coordinates.
type LocusKit struct {
	ID       LocusKitID
	MaxCells int         // maximum cell count (must be ≤ 64 — bounded by low-6-bit addressing)
	Layout   LocusLayout // cell → (px, pz, angle)
}

var gLocusRegistry struct {
	sync.RWMutex
	Lookup map[LocusKitID]*LocusKit
}

// RegisterLocusKit registers the given LocusKit so it can be retrieved via GetLocusKit.
// Safe to call from init().  Registering the same kit pointer twice is a no-op.
func RegisterLocusKit(kit *LocusKit) error {
	var err error
	gLocusRegistry.Lock()
	if gLocusRegistry.Lookup == nil {
		gLocusRegistry.Lookup = map[LocusKitID]*LocusKit{}
	}
	existing := gLocusRegistry.Lookup[kit.ID]
	if existing == nil {
		gLocusRegistry.Lookup[kit.ID] = kit
	} else if existing != kit {
		err = fmt.Errorf("LocusKit %d is already registered", kit.ID)
	}
	gLocusRegistry.Unlock()
	return err
}

// GetLocusKit fetches a registered LocusKit by its ID.
// Returns an error if the kit has not been registered.
func GetLocusKit(id LocusKitID) (*LocusKit, error) {
	gLocusRegistry.RLock()
	kit := gLocusRegistry.Lookup[id]
	gLocusRegistry.RUnlock()
	if kit == nil {
		return nil, fmt.Errorf("LocusKit %d not found", id)
	}
	return kit, nil
}

func init() {
	_ = RegisterLocusKit(&LocusKit{
		ID:       LocusKit_HexSpiral,
		MaxCells: locusHexCells,
		Layout:   LayoutHexSpiral,
	})
	_ = RegisterLocusKit(&LocusKit{
		ID:       LocusKit_Linear,
		MaxCells: locusSpan,
		Layout:   LayoutLinear,
	})
	_ = RegisterLocusKit(&LocusKit{
		ID:       LocusKit_RadialRing,
		MaxCells: locusHexCells,
		Layout:   LayoutRadialRing,
	})
	_ = RegisterLocusKit(&LocusKit{
		ID:       LocusKit_Torus,
		MaxCells: locusSpan,
		Layout:   LayoutTorus,
	})
}
