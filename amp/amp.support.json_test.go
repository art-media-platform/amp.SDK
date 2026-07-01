package amp_test

import (
	"encoding/json"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

func TestTagJSONBase32(t *testing.T) {
	uid := tag.NameFrom("spaces.plan.tools").ID
	in := &amp.Tag{UID_0: uid[0], UID_1: uid[1], ContentTypeRaw: "image/png", URI: "amp://x/y"}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"UID":"`+uid.Base32()+`"`) {
		t.Fatalf("Tag JSON not base32: %s", data)
	}
	if strings.Contains(string(data), "UID_0") || strings.Contains(string(data), "UID0") {
		t.Fatalf("Tag JSON leaked fixed64 fields: %s", data)
	}

	out := &amp.Tag{}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatal(err)
	}
	if out.UID_0 != in.UID_0 || out.UID_1 != in.UID_1 || out.ContentTypeRaw != in.ContentTypeRaw || out.URI != in.URI {
		t.Fatalf("Tag round-trip mismatch: %+v", out)
	}
}

func TestCrateRefJSONBase32(t *testing.T) {
	uid := tag.NameFrom("crate.blob").ID
	in := &amp.CrateRef{CrateURI: "asset:pub/crate", BlobID_0: uid[0], BlobID_1: uid[1]}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"BlobID":"`+uid.Base32()+`"`) {
		t.Fatalf("CrateRef JSON not base32: %s", data)
	}

	out := &amp.CrateRef{}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatal(err)
	}
	if out.BlobID_0 != in.BlobID_0 || out.BlobID_1 != in.BlobID_1 || out.CrateURI != in.CrateURI {
		t.Fatalf("CrateRef round-trip mismatch: %+v", out)
	}
}

func TestAddressJSONPackedBase32(t *testing.T) {
	node := tag.NameFrom("the.node").ID
	attr := tag.NameFrom("the.attr").ID
	item := tag.NameFrom("the.item").ID
	edit := tag.NowID()
	planet := tag.NameFrom("the.planet").ID

	cases := []struct {
		name string
		in   *amp.Address
	}{
		{"element-3", &amp.Address{
			NodeID_0: node[0], NodeID_1: node[1],
			AttrID_0: attr[0], AttrID_1: attr[1],
			ItemID_0: item[0], ItemID_1: item[1],
		}},
		{"address-4", &amp.Address{
			NodeID_0: node[0], NodeID_1: node[1],
			AttrID_0: attr[0], AttrID_1: attr[1],
			ItemID_0: item[0], ItemID_1: item[1],
			EditID_0: edit[0], EditID_1: edit[1],
		}},
		{"full-5", &amp.Address{
			PlanetID_0: planet[0], PlanetID_1: planet[1],
			NodeID_0: node[0], NodeID_1: node[1],
			AttrID_0: attr[0], AttrID_1: attr[1],
			ItemID_0: item[0], ItemID_1: item[1],
			EditID_0: edit[0], EditID_1: edit[1],
		}},
	}

	for _, tc := range cases {
		data, err := json.Marshal(tc.in)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		// A single JSON string, no object braces, no fixed64 fields.
		if strings.ContainsAny(string(data), "{}") || strings.Contains(string(data), "NodeID") {
			t.Fatalf("%s: Address not a packed string: %s", tc.name, data)
		}

		out := &amp.Address{}
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if !proto.Equal(out, tc.in) {
			t.Fatalf("%s: Address round-trip mismatch:\n got %+v\nwant %+v", tc.name, out, tc.in)
		}
	}

	// Nil address → empty string.
	data, _ := json.Marshal(&amp.Address{})
	if string(data) != `""` {
		t.Fatalf("nil Address should marshal to empty string, got %s", data)
	}
}
