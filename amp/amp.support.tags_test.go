package amp

import (
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/proto"
)

// TestTagsJSON covers the document-vs-tree JSON: a pure-text document marshals to a clean
// ContentType→Text map (the SPA reads body["text/html"]) and round-trips; a richer tree stays
// faithful (Head/SubTags/Children).  Storage is proto, untouched — this is the web-wire only.
func TestTagsJSON(t *testing.T) {
	doc := NewTags(TagText("text/html", "<p>hi</p>"), TagText("text/markdown", "hi"))
	js, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var shape map[string]string // a tree {"Head":…} would fail to parse here, proving the flat doc map
	if err := json.Unmarshal(js, &shape); err != nil {
		t.Fatalf("document JSON is not a ContentType map: %s", js)
	}
	if len(shape) != 2 || shape["text/html"] != "<p>hi</p>" || shape["text/markdown"] != "hi" {
		t.Fatalf("document map = %v", shape)
	}
	var back Tags
	if err := json.Unmarshal(js, &back); err != nil {
		t.Fatal(err)
	}
	if back.TextByContentType("text/html") != "<p>hi</p>" || back.TextByContentType("text/markdown") != "hi" {
		t.Fatalf("round-trip lost leaves: %+v", &back)
	}

	// A leaf carrying a URI (not pure text) + a Child → faithful form, structure preserved.
	tree := NewTags(&Tag{URI: "amp://x", ContentTypeRaw: "text/html"}).AddChild(NewTags(TagText("text/plain", "c")))
	js, _ = json.Marshal(tree)
	var backTree Tags
	if err := json.Unmarshal(js, &backTree); err != nil {
		t.Fatal(err)
	}
	if backTree.Head == nil || backTree.Head.URI != "amp://x" || len(backTree.Children) != 1 {
		t.Fatalf("tree round-trip lost structure: %s", js)
	}
}

// TestTagsErgonomics covers the content-model builder/accessor layer: a document is an
// amp.Tags branch whose leaves self-declare ContentType, built + read without pointer-chasing.
func TestTagsErgonomics(t *testing.T) {
	// A multi-representation document body: html (Head) + markdown (SubTag).
	body := NewTags(
		TagText("text/html", "<p>hi</p>"),
		TagText("text/markdown", "hi"),
	)

	if got := body.TextByContentType("text/html"); got != "<p>hi</p>" {
		t.Fatalf("html leaf: got %q", got)
	}
	if got := body.TextByContentType("text/markdown"); got != "hi" {
		t.Fatalf("markdown leaf: got %q", got)
	}
	if body.ByContentType("text/plain") != nil {
		t.Fatal("absent ContentType must return nil")
	}

	// Walk reaches every leaf (Head + SubTags + Children).
	body.AddSubTag(TagText("text/plain", "hi")).
		AddChild(NewTags(TagText("application/json", `{"k":1}`)))
	var leaves int
	body.Walk(func(*Tag) { leaves++ })
	if leaves != 4 { // html, markdown, plain, json-child-head
		t.Fatalf("walk visited %d leaves, want 4", leaves)
	}
	if got := body.TextByContentType("text/plain"); got != "hi" {
		t.Fatalf("AddSubTag chain: got %q", got)
	}

	// Nil-safety: accessors on a nil Tags must not panic.
	var nilTags *Tags
	if nilTags.ByContentType("text/html") != nil || nilTags.TextByContentType("x") != "" {
		t.Fatal("nil Tags accessors must be nil/empty")
	}
	nilTags.Walk(func(*Tag) { t.Fatal("nil Tags must not visit") })
}

// TestPlainTextInterning covers the text/plain default collapse and case canonization: TagText
// lower-cases the type and strips text/plain (any case) to an empty ContentTypeRaw, while
// Tag.ContentType() resolves ContentTypeRaw into a portable, lower-cased media type.
func TestPlainTextInterning(t *testing.T) {
	// Authoring: text/plain (any case) and "" collapse to an empty ContentTypeRaw.
	for _, ct := range []string{"text/plain", "TEXT/PLAIN", "Text/Plain", ""} {
		leaf := TagText(ct, "hi")
		if leaf.ContentTypeRaw != "" {
			t.Fatalf("TagText(%q) kept ContentTypeRaw %q, want empty", ct, leaf.ContentTypeRaw)
		}
		if got := leaf.ContentType(); got != DefaultContentType {
			t.Fatalf("TagText(%q).ContentType() = %q, want %q", ct, got, DefaultContentType)
		}
	}

	// A non-plain type is canonized to lower case on the way in and resolves to itself.
	html := TagText("TEXT/HTML", "<p>hi</p>")
	if html.ContentTypeRaw != "text/html" || html.ContentType() != "text/html" {
		t.Fatalf("html leaf: raw=%q ct=%q", html.ContentTypeRaw, html.ContentType())
	}

	// A leaf that stored a mixed-case type directly still resolves to canonical lower case, and
	// is reachable case-insensitively by ByContentType.
	stored := &Tag{ContentTypeRaw: "Image/PNG", Text: "x"}
	if stored.ContentType() != "image/png" {
		t.Fatalf("stored mixed-case not canonized: ct=%q", stored.ContentType())
	}
	if NewTags(stored).ByContentType("IMAGE/png") != stored {
		t.Fatal("ByContentType must match case-insensitively")
	}

	// Nil-safety.
	var nilLeaf *Tag
	if nilLeaf.ContentType() != "" {
		t.Fatal(`nil leaf: want ContentType()=""`)
	}
}

// TestTagTextWireSize locks the win: an interned text/plain leaf carries no ContentTypeRaw on
// the wire — exactly a bare-Text tag's size, and 13 bytes smaller than storing the literal.
func TestTagTextWireSize(t *testing.T) {
	interned, err := proto.Marshal(TagText("text/plain", "hello"))
	if err != nil {
		t.Fatal(err)
	}
	bare, err := proto.Marshal(&Tag{Text: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := proto.Marshal(&Tag{ContentTypeRaw: "text/plain", Text: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if len(interned) != len(bare) {
		t.Fatalf("interned text/plain = %d B, want bare-Text size %d B", len(interned), len(bare))
	}
	if delta := len(stored) - len(interned); delta != 13 {
		t.Fatalf("text/plain string cost = %d B (2 B tag + 1 B len + 10 B ascii), want 13", delta)
	}
}
