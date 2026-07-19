package std

import (
	"sync"
	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// The identical-policy re-registration no-op falsifier: a redundant
// RegisterAttr publishes nothing — the snapshot pointer is unchanged — while
// a DIFFERENT policy keeps the write-once refusal and also publishes nothing.
func TestRegistryRedundantRegisterNoOp(t *testing.T) {
	reg := NewRegistry().(*registry)

	def := amp.AttrDef{
		Name:      tag.Name{}.With("test.registry.snapshot.Tag"),
		Prototype: &amp.Tag{},
		EditFlow:  amp.EditFlow_Tape,
	}
	if err := reg.RegisterAttr(def); err != nil {
		t.Fatalf("RegisterAttr failed: %v", err)
	}
	before := reg.snap.Load()

	// Identical resolved policy: no-op before the clone — same snapshot.
	if err := reg.RegisterAttr(def); err != nil {
		t.Fatalf("identical re-registration must no-op, got: %v", err)
	}
	if reg.snap.Load() != before {
		t.Fatal("identical re-registration published a new snapshot")
	}

	// Different policy: write-once refusal, and still no publish.
	changed := def
	changed.EditFlow = amp.EditFlow_Fold
	changed.RetainEdits = 3
	if err := reg.RegisterAttr(changed); err == nil {
		t.Fatal("differing storage policy must be refused")
	}
	if reg.snap.Load() != before {
		t.Fatal("refused re-registration published a new snapshot")
	}

	// The surviving def is the original.
	found, ok := reg.FindAttr(def.ID)
	if !ok || found.EditFlow != amp.EditFlow_Tape {
		t.Fatal("original def lost after redundant/refused re-registrations")
	}
}

// Concurrent FindAttr / NewValue against live registration — the lock-free
// read path under -race.
func TestRegistryConcurrentReadWrite(t *testing.T) {
	reg := NewRegistry().(*registry)

	names := make([]tag.Name, 64)
	for i := range names {
		names[i] = tag.Name{}.With("test.registry.concurrent." + string(rune('a'+i/26)) + string(rune('a'+i%26)) + ".Tag")
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for _, name := range names {
			err := reg.RegisterAttr(amp.AttrDef{
				Name:      name,
				Prototype: &amp.Tag{},
			})
			if err != nil {
				t.Errorf("RegisterAttr failed: %v", err)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for range [4]int{} {
			for _, name := range names {
				if def, ok := reg.FindAttr(name.ID); ok {
					if _, err := reg.NewValue(def.ID); err != nil {
						t.Errorf("NewValue failed: %v", err)
					}
				}
			}
		}
	}()
	wg.Wait()

	for _, name := range names {
		if _, ok := reg.FindAttr(name.ID); !ok {
			t.Fatalf("attr %q missing after concurrent registration", name.Canonic())
		}
	}
}
