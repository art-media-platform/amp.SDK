package std_test

import (
	"reflect"
	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/amp/std"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

func TestRegistry(t *testing.T) {
	reg := std.NewRegistry()
	someAttr := tag.Name{}.With("hello sailor")
	spec := amp.AttrDef{
		Name:      someAttr.With("av.Hello.World.Tag"),
		Prototype: &amp.Tag{},
	}

	{
		err := reg.RegisterAttr(spec)
		if err != nil {
			t.Fatalf("RegisterAttr failed: %v", err)
		}
		elem, err := reg.MakeValue(spec.ID)
		if err != nil {
			t.Fatalf("MakeValue failed: %v", err)
		}
		if spec.Canonic != someAttr.Canonic+".av.hello.world.tag" {
			t.Fatal("RegisterAttr failed")
		}
		if reflect.TypeOf(elem) != reflect.TypeOf(&amp.Tag{}) {
			t.Fatalf("MakeValue returned wrong type: %v", reflect.TypeOf(elem))
		}
	}

	if spec.ID != (tag.Name{}.With("hello.sailor.World.Tag.Hello.av")).ID {
		t.Fatalf("tag.With failed")
	}
	alias := someAttr.With("av").With("World.Hello.Tag")
	if spec.ID != alias.ID {
		t.Fatalf("tag.With failed")
	}
	if str := spec.ID.AsLabel(); str != "227W6E" {
		t.Fatalf("unexpected spec.ID: %v", str)
	}
	if (tag.UID{}).Base32() != "0" {
		t.Fatalf("tag.Name{}.Base32() failed")
	}
	base32 := spec.ID.Base32()
	if base32 != "5UZ9YW2T5KF8YJYV4C2Y227W6E" {
		t.Errorf("tag.UID.Base32() failed: %v", base32)
	}
	if str := spec.ID.Base16(); str != "0xBAFA7DC164B2723D1F6C8B178423F0CD" {
		t.Errorf("tag.UID.Base16() failed: %v", str)
	}

}
