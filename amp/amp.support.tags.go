package amp

import (
	"strings"

	"github.com/art-media-platform/amp.SDK/stdlib/status"
)

// Tag / Tags ergonomics — the builder + accessor layer for the content model where a
// value's DOCUMENT is an amp.Tags tree of amp.Tag leaves, each leaf self-declaring its
// IANA ContentType (the renderer / sanitizer reads the type off the leaf rather than
// trusting a field name).  The wire + registry already carry Tags as any proto; these
// helpers remove the pointer-chasing so a Tags document reads + builds like a struct.

// TagText returns a leaf Tag carrying UTF-8 text of the given IANA content type — the
// universal context-leaf: a string that self-declares what it is ("text/html",
// "text/markdown", "application/json", …).  The type is canonized to lower case and the
// text/plain default is stripped to "", so a plain-text leaf costs 0 wire bytes.  Read the
// effective type back through Tag.ContentType(), never off the raw ContentTypeRaw field.
func TagText(contentType, text string) *Tag {
	contentType = strings.ToLower(contentType)
	if contentType == DefaultContentType {
		contentType = ""
	}
	return &Tag{
		ContentTypeRaw: contentType,
		Text:           text,
	}
}

// TagFromData returns a leaf Tag carrying an inline binary payload of the given IANA media
// type — the attachment leaf (a verifier record, cover art, a glyph).  The type is canonized
// to lower case and is REQUIRED (Tag.Validate); unlike TagText it is never stripped, because
// an empty ContentTypeRaw on a Data leaf is a refusal, not a default (AOM SD-content-substrate.md §3.6).
func TagFromData(contentType string, data []byte) *Tag {
	return &Tag{
		ContentTypeRaw: strings.ToLower(contentType),
		Data:           data,
	}
}

// ContentType resolves ContentTypeRaw into a portable, canonical IANA media type: lower-cased
// (media types are case-insensitive) with the empty default resolved to DefaultContentType
// (text/plain).  It is the stable accessor — callers read the type through it, never off the
// raw ContentTypeRaw field — so the raw representation can evolve (e.g. an interned type code)
// without touching call sites.  Nil-safe (nil → "").
func (tagValue *Tag) ContentType() string {
	if tagValue == nil {
		return ""
	}
	if tagValue.ContentTypeRaw == "" {
		return DefaultContentType
	}
	return strings.ToLower(tagValue.ContentTypeRaw)
}

// ErrTagNoContentType flags a leaf carrying Data with no ContentTypeRaw.  An attachment with no
// media type is unrenderable — nothing downstream can decide how to read the bytes — so it is
// refused where the value is authored rather than stored and served as unusable payload.
var ErrTagNoContentType = status.Code_BadRequest.Error("amp.Tag: Data requires ContentTypeRaw")

// Validate reports whether a leaf's field combination is legal — the ONE statement of Tag
// syntax (AOM SD-content-substrate.md §3.6).  Data (an inline attachment) requires
// ContentTypeRaw.  URI and Data together are legal and expected — the post-fetch cache, where
// URI is the object's location and Data a copy materialized inline — and carry no precedence
// rule.  Nil-safe.
func (tagValue *Tag) Validate() error {
	if tagValue == nil || len(tagValue.Data) == 0 {
		return nil
	}
	if tagValue.ContentTypeRaw == "" {
		return ErrTagNoContentType
	}
	return nil
}

// Validate checks every leaf of the tree — Head, SubTags, and Children alike, since an
// attachment is legal at any depth (AOM SD-content-substrate.md §3.6) — and
// returns the first refusal.  Nil-safe.
func (tagsValue *Tags) Validate() error {
	var refused error
	tagsValue.Walk(func(leaf *Tag) {
		if refused == nil {
			refused = leaf.Validate()
		}
	})
	return refused
}

// NewTags builds a Tags branch: head identifies the branch, subTags amplify it (sibling
// leaves — e.g. the alternate representations of one document).  Children (sequential
// sub-trees) attach via AddChild.
func NewTags(head *Tag, subTags ...*Tag) *Tags {
	return &Tags{
		Head:    head,
		SubTags: subTags,
	}
}

// AddSubTag appends an amplifying leaf and returns t (chainable).
func (tagsValue *Tags) AddSubTag(sub *Tag) *Tags {
	tagsValue.SubTags = append(tagsValue.SubTags, sub)
	return tagsValue
}

// AddChild appends a sequential sub-tree and returns t (chainable).
func (tagsValue *Tags) AddChild(child *Tags) *Tags {
	tagsValue.Children = append(tagsValue.Children, child)
	return tagsValue
}

// ByContentType returns the first leaf (Head, then SubTags) whose ContentType matches,
// or nil — the accessor a document-as-Tags value reads through, e.g.
// post.Body.ByContentType("text/html").  Nil-safe.
func (tagsValue *Tags) ByContentType(contentType string) *Tag {
	if tagsValue == nil {
		return nil
	}
	contentType = strings.ToLower(contentType)
	if tagsValue.Head != nil && tagsValue.Head.ContentType() == contentType {
		return tagsValue.Head
	}
	for _, sub := range tagsValue.SubTags {
		if sub.ContentType() == contentType {
			return sub
		}
	}
	return nil
}

// TextByContentType returns the Text of the first leaf matching contentType, or "".
func (tagsValue *Tags) TextByContentType(contentType string) string {
	if leaf := tagsValue.ByContentType(contentType); leaf != nil {
		return leaf.Text
	}
	return ""
}

// Walk visits Head, each SubTag, then recurses into Children, depth-first — the flatten
// used by renderers and the ACC content gate to reach every leaf.  Nil-safe.
func (tagsValue *Tags) Walk(visit func(*Tag)) {
	if tagsValue == nil {
		return
	}
	if tagsValue.Head != nil {
		visit(tagsValue.Head)
	}
	for _, sub := range tagsValue.SubTags {
		visit(sub)
	}
	for _, child := range tagsValue.Children {
		child.Walk(visit)
	}
}
