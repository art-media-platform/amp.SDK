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
	uid := tag.HashName("spaces.plan.tools").ID
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
	uid := tag.HashName("crate.blob").ID
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

func TestBlobRefJSONBase32(t *testing.T) {
	blobID := tag.HashName("photo.blob").ID
	in := &amp.BlobRef{
		PlanetID_0: 7, // dropped on the JSON wire — resolution is by channel planet
		PlanetID_1: 9,
		Hash_0:     blobID[0],
		Hash_1:     blobID[1],
		Hash_2:     3,
		Hash_3:     4,
		AssetTag: &amp.Tag{
			UID_0:          blobID[0],
			UID_1:          blobID[1],
			ContentTypeRaw: "image/png",
			Text:           "photo.png",
			I:              512,
			Units:          amp.Units_Bytes,
		},
		BlobTag: &amp.Tag{
			UID_0: blobID[0],
			UID_1: blobID[1],
			I:     512,
			Units: amp.Units_Bytes,
		},
	}

	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"BlobID":"`+blobID.Base32()+`"`) {
		t.Fatalf("BlobRef JSON not base32: %s", data)
	}
	for _, leaked := range []string{"PlanetID", "Hash_0", "Hash0", "HashKit"} {
		if strings.Contains(string(data), leaked) {
			t.Fatalf("BlobRef JSON leaked %s: %s", leaked, data)
		}
	}
	if !strings.Contains(string(data), `"ContentType":"image/png"`) ||
		!strings.Contains(string(data), `"Name":"photo.png"`) ||
		!strings.Contains(string(data), `"Size":512`) {
		t.Fatalf("BlobRef JSON missing render metadata: %s", data)
	}
	if strings.Contains(string(data), "EpochID") { // public blob → omitempty drops it
		t.Fatalf("BlobRef JSON carries an empty EpochID: %s", data)
	}

	// Round-trip restores the compact identity: storage UID + render metadata
	// (the full hash / HashKitID / PlanetID are not representable — see blobRefJSON).
	out := &amp.BlobRef{}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatal(err)
	}
	if out.StorageUID() != in.StorageUID() {
		t.Fatalf("BlobRef round-trip StorageUID mismatch: %+v", out)
	}
	if out.AssetTag == nil || out.AssetTag.ContentType() != "image/png" ||
		out.AssetTag.Text != "photo.png" || out.AssetTag.I != 512 {
		t.Fatalf("BlobRef round-trip AssetTag mismatch: %+v", out.AssetTag)
	}
	if out.PlanetID_0 != 0 || out.PlanetID_1 != 0 {
		t.Fatalf("BlobRef round-trip resurrected a PlanetID: %+v", out)
	}

	// The empty ref marshals to {} and round-trips empty.
	empty, err := json.Marshal(&amp.BlobRef{})
	if err != nil {
		t.Fatal(err)
	}
	if string(empty) != "{}" {
		t.Fatalf("empty BlobRef JSON = %s, want {}", empty)
	}
	blank := &amp.BlobRef{}
	if err := json.Unmarshal(empty, blank); err != nil {
		t.Fatal(err)
	}
	if blank.AssetTag != nil || blank.BlobTag != nil || blank.StorageUID().IsSet() {
		t.Fatalf("empty BlobRef round-trip not empty: %+v", blank)
	}
}

func TestAddressJSONPackedBase32(t *testing.T) {
	node := tag.HashName("the.node").ID
	attr := tag.HashName("the.attr").ID
	item := tag.HashName("the.item").ID
	edit := tag.NowID()
	planet := tag.HashName("the.planet").ID

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
