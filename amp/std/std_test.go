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
		elem, err := reg.NewValue(spec.ID)
		if err != nil {
			t.Fatalf("Make failed: %v", err)
		}
		if spec.Canonic != someAttr.Canonic+".av.hello.world.tag" {
			t.Fatal("RegisterAttr failed")
		}
		if reflect.TypeOf(elem) != reflect.TypeOf(&amp.Tag{}) {
			t.Fatalf("Make returned wrong type: %v", reflect.TypeOf(elem))
		}
	}

	// Order is significant now (atomic hash of the canonic string): the same
	// literals in the same order reproduce the UID; the invariant ties ID to
	// the canonic string.
	if spec.ID != (tag.Name{}.With("hello.sailor.av.Hello.World.Tag")).ID {
		t.Fatalf("tag.With failed")
	}
	if spec.ID != tag.UID_HashLiteral([]byte(spec.Canonic)) {
		t.Fatalf("spec.ID != HashLiteral(Canonic)")
	}
	alias := someAttr.With("av").With("Hello.World.Tag")
	if spec.ID != alias.ID {
		t.Fatalf("tag.With failed")
	}
	if str := spec.ID.AsLabel(); str != "2U..XHJF" {
		t.Fatalf("unexpected spec.ID: %v", str)
	}
	if (tag.UID{}).Base32() != "0" {
		t.Fatalf("tag.Name{}.Base32() failed")
	}
	base32 := spec.ID.Base32()
	if base32 != "2UE627EB7UT34RQH99825EXHJF" {
		t.Errorf("tag.UID.Base32() failed: %v", base32)
	}
	if str := spec.ID.Base16(); str != "0x5A698476A8FAC8C97B4129408ADEC22E" {
		t.Errorf("tag.UID.Base16() failed: %v", str)
	}

}
