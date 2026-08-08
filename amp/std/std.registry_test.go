package std

import (
	"reflect"
	"strings"
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

// The generated-registration round trip (B-attr-autoreg): every attr the
// forge registration emitted is live in the process registry, count-exact
// and §4.8-conformant, with golden UIDs asserted as BYTES.
func TestGeneratedAttrRegistration(t *testing.T) {
	// amp.std.consts.sdl declares 61 registrable attrs (trailing message-type
	// word, ZO §4.8); std.terminal.go registers 2 more at use-site.  A count
	// drift means a registration was added or lost — both are conscious edits.
	const generatedAttrs = 61
	const useSiteAttrs = 2

	count := 0
	Registry().EnumAttrs(func(def amp.AttrDef) bool {
		count++
		// §4.8 sweep: the trailing name word IS the reflected prototype name.
		typeName := reflect.TypeOf(def.Prototype).Elem().Name()
		text := def.Name.Text
		if lastDot := strings.LastIndexByte(text, '.'); lastDot >= 0 {
			text = text[lastDot+1:]
		}
		if text != typeName {
			t.Errorf("attr %q: trailing word %q != prototype type %q", def.Name.Text, text, typeName)
		}
		return true
	})
	if count != generatedAttrs+useSiteAttrs {
		t.Fatalf("registry holds %d attrs, want %d generated + %d use-site", count, generatedAttrs, useSiteAttrs)
	}

	// The declared-flag → EditFlow mapping (ZO §4.8 declared flags):
	// series attrs are the ITEM axis and fold — their consumers bind
	// best/latest per item (astar 08-08: "we just want the best /
	// latest"; 280 D-series-editflow: a tape lane is a NEW attr, never
	// a re-class).
	flagged := []struct {
		attr tag.Name
		flow amp.EditFlow
	}{
		{Attr.SeriesTRS, amp.EditFlow_Fold},
		{Attr.SeriesLabels, amp.EditFlow_Fold},
		{Attr.SeriesAssetTag, amp.EditFlow_Fold},
		{Attr.SeriesHeadLink, amp.EditFlow_Fold},
		{Attr.SeriesLinkTree, amp.EditFlow_Fold},
		{Attr.ChannelPropertySeries, amp.EditFlow_Fold},
	}
	for _, f := range flagged {
		def, ok := Registry().FindAttr(f.attr.ID)
		if !ok {
			t.Errorf("attr %q: not registered", f.attr.Text)
			continue
		}
		if def.EditFlow != f.flow {
			t.Errorf("attr %q: EditFlow %v != declared %v", f.attr.Text, def.EditFlow, f.flow)
		}
	}

	// Golden fixtures — identity is BYTES (fold parity between forge codegen
	// and the runtime tag fold).
	golden := []struct {
		attr tag.Name
		uid  tag.UID
		flow amp.EditFlow
	}{
		{Attr.AppState, tag.UID{0x6CEB3696AB78B359, 0x08CF79D767A904E8}, amp.EditFlow_Fold},
		{Attr.SeriesTRS, tag.UID{0x6BECC785388E3D9E, 0x25958F2CECD0021A}, amp.EditFlow_Fold},
		{Attr.SessionStatus, tag.UID{0x7FB381BC8DB19B28, 0xE3EC642BF561552B}, amp.EditFlow_Fold},
	}
	for _, g := range golden {
		if g.attr.ID != g.uid {
			t.Errorf("attr %q: UID %v != golden %v", g.attr.Text, g.attr.ID, g.uid)
		}
		def, ok := Registry().FindAttr(g.uid)
		if !ok {
			t.Errorf("attr %q: not registered", g.attr.Text)
			continue
		}
		if def.EditFlow != g.flow {
			t.Errorf("attr %q: EditFlow %v != %v", g.attr.Text, def.EditFlow, g.flow)
		}
		if _, err := Registry().NewValue(g.uid); err != nil {
			t.Errorf("attr %q: NewValue: %v", g.attr.Text, err)
		}
	}
}
