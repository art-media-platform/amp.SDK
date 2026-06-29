package amp

import (
	"encoding/json"
	"testing"
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
	tree := NewTags(&Tag{URI: "amp://x", ContentType: "text/html"}).AddChild(NewTags(TagText("text/plain", "c")))
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
