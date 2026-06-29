package amp

import "testing"

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
