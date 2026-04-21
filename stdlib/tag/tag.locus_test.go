package tag

import (
	"math"
	"testing"
)

func TestLocusBaseAndCell(t *testing.T) {
	base := UID{0xDEAD, 0xBEEF00}
	if base.LocusBase() != base {
		t.Fatal("base with low 6 bits = 0 should be its own LocusBase")
	}

	for cell := 0; cell < 37; cell++ {
		uid := base.WithCell(cell)
		if uid.LocusCell() != cell {
			t.Fatalf("cell %d: got LocusCell %d", cell, uid.LocusCell())
		}
		if uid.LocusBase() != base {
			t.Fatalf("cell %d: LocusBase mismatch", cell)
		}
	}
}

func TestLocusMatch(t *testing.T) {
	base := UID{0x1234, 0xABC0} // low 6 bits = 0
	for cell := 0; cell < 64; cell++ {
		uid := base.WithCell(cell)
		if !base.LocusMatch(uid) {
			t.Fatalf("cell %d should match base", cell)
		}
	}

	other := UID{0x1234, 0xAB00} // different base
	if base.LocusMatch(other) {
		t.Fatal("different base should not match")
	}

	diffHi := UID{0x9999, 0xABC0}
	if base.LocusMatch(diffHi) {
		t.Fatal("different UID[0] should not match")
	}
}

func TestHexSpiralCenter(t *testing.T) {
	px, pz, _ := LayoutHexSpiral(0, 1.0)
	if px != 0 || pz != 0 {
		t.Fatalf("center should be (0,0), got (%f,%f)", px, pz)
	}
}

func TestHexSpiralRing1(t *testing.T) {
	px, pz, _ := LayoutHexSpiral(1, 2.0)
	if math.Abs(px-2.0) > 1e-9 || math.Abs(pz) > 1e-9 {
		t.Fatalf("cell 1 at spacing 2 should be (2,0), got (%f,%f)", px, pz)
	}
}

func TestLinearLayout(t *testing.T) {
	for cell := 0; cell < 7; cell++ {
		px, pz, _ := LayoutLinear(cell, 1.5)
		expected := float64(cell) * 1.5
		if math.Abs(px-expected) > 1e-9 || pz != 0 {
			t.Fatalf("cell %d: expected (%f,0), got (%f,%f)", cell, expected, px, pz)
		}
	}
}

func TestRadialRingLayout(t *testing.T) {
	// Cell 0 at center
	px, pz, _ := LayoutRadialRing(0, 1.0)
	if px != 0 || pz != 0 {
		t.Fatalf("cell 0 should be (0,0), got (%f,%f)", px, pz)
	}

	// Ring 1 cells 1-6 should all be at radius = spacing
	for cell := 1; cell <= 6; cell++ {
		px, pz, _ := LayoutRadialRing(cell, 3.0)
		radius := math.Sqrt(px*px + pz*pz)
		if math.Abs(radius-3.0) > 1e-9 {
			t.Fatalf("cell %d: expected radius 3, got %f", cell, radius)
		}
	}

	// Ring 2 cells 7-18 should be at radius = 2*spacing
	for cell := 7; cell <= 18; cell++ {
		px, pz, _ := LayoutRadialRing(cell, 2.0)
		radius := math.Sqrt(px*px + pz*pz)
		if math.Abs(radius-4.0) > 1e-9 {
			t.Fatalf("cell %d: expected radius 4, got %f", cell, radius)
		}
	}
}
