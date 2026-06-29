package amp

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/art-media-platform/amp.SDK/stdlib/encode"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// JSON wire convention for UID-bearing proto types.
//
// Transport rule: string transport (JSON) carries UIDs as base32; binary
// transport (protobuf) carries the native fixed64 pair.  proto.Marshal is
// untouched by the marshalers below — only encoding/json paths see base32.
// protojson ignores json.Marshaler, so webapi endpoints that want this
// shape must decode/encode with encoding/json, not protojson.
//
// Field keys are PascalCase, matching the Go/C# identifiers (one identifier
// set across all platforms).

// uidToBase32 renders a UID pair as its base32 short form, or "" when nil so
// that an omitempty field drops cleanly.
func uidToBase32(x0, x1 uint64) string {
	id := tag.UID{x0, x1}
	if id.IsNil() {
		return ""
	}
	return id.Base32()
}

// uidFromBase32 parses a base32 UID; "" decodes to the nil UID.
func uidFromBase32(str string) (tag.UID, error) {
	if str == "" {
		return tag.UID{}, nil
	}
	return tag.UID_ParseBase32(str)
}

// --- amp.WithdrawReason ---------------------------------------------------
//
// On the JSON wire a reason rides as its self-documenting enum name ("Consent",
// "Outdated", …) rather than an integer.  UnmarshalJSON accepts either the name
// or the numeric value.

func (reason WithdrawReason) MarshalJSON() ([]byte, error) {
	if name, ok := WithdrawReason_name[int32(reason)]; ok {
		return json.Marshal(name)
	}
	return json.Marshal(int32(reason))
}

func (reason *WithdrawReason) UnmarshalJSON(data []byte) error {
	name := ""
	if err := json.Unmarshal(data, &name); err == nil {
		value, ok := WithdrawReason_value[name]
		if !ok {
			return fmt.Errorf("amp.WithdrawReason: unknown reason %q", name)
		}
		*reason = WithdrawReason(value)
		return nil
	}
	value := int32(0)
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*reason = WithdrawReason(value)
	return nil
}

// --- amp.TrustState -------------------------------------------------------
//
// A resolve verdict rides the JSON wire as its self-documenting enum name
// ("Unchecked", "Verified", "Refuted") rather than an integer, so a consumer
// branches on a stable string instead of a magic number — matching the
// WithdrawReason convention above.  UnmarshalJSON accepts either form.

func (trust TrustState) MarshalJSON() ([]byte, error) {
	if name, ok := TrustState_name[int32(trust)]; ok {
		return json.Marshal(name)
	}
	return json.Marshal(int32(trust))
}

func (trust *TrustState) UnmarshalJSON(data []byte) error {
	name := ""
	if err := json.Unmarshal(data, &name); err == nil {
		value, ok := TrustState_value[name]
		if !ok {
			return fmt.Errorf("amp.TrustState: unknown state %q", name)
		}
		*trust = TrustState(value)
		return nil
	}
	value := int32(0)
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*trust = TrustState(value)
	return nil
}

// --- amp.Tag --------------------------------------------------------------

type tagJSON struct {
	UID         string `json:"UID,omitempty"`
	I           int64  `json:"I,omitempty"`
	J           int64  `json:"J,omitempty"`
	K           int64  `json:"K,omitempty"`
	Units       Units  `json:"Units,omitempty"`
	ContentType string `json:"ContentType,omitempty"`
	URI         string `json:"URI,omitempty"`
	Text        string `json:"Text,omitempty"`
}

func (tagValue *Tag) MarshalJSON() ([]byte, error) {
	return json.Marshal(tagJSON{
		UID:         uidToBase32(tagValue.UID_0, tagValue.UID_1),
		I:           tagValue.I,
		J:           tagValue.J,
		K:           tagValue.K,
		Units:       tagValue.Units,
		ContentType: tagValue.ContentType,
		URI:         tagValue.URI,
		Text:        tagValue.Text,
	})
}

func (tagValue *Tag) UnmarshalJSON(data []byte) error {
	wire := tagJSON{}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	uid, err := uidFromBase32(wire.UID)
	if err != nil {
		return err
	}
	tagValue.UID_0, tagValue.UID_1 = uid[0], uid[1]
	tagValue.I, tagValue.J, tagValue.K = wire.I, wire.J, wire.K
	tagValue.Units = wire.Units
	tagValue.ContentType, tagValue.URI, tagValue.Text = wire.ContentType, wire.URI, wire.Text
	return nil
}

// --- amp.Tags ---------------------------------------------------------------
//
// A "document" Tags — leaves only (no Children), each a pure ContentType+Text leaf with a
// distinct ContentType — marshals to a clean { "<ContentType>": "<Text>", … } map, so a web
// client reads body["text/html"].  Any richer tree (Children, or a leaf carrying a UID / URI /
// scalar) marshals to the faithful { Head, SubTags, Children } form (identical to the prior
// default).  Unmarshal accepts both: the faithful form is detected by its Head/SubTags/Children
// keys (a media type is never one), otherwise the object is a ContentType→Text document.

type tagsTreeJSON struct {
	Head     *Tag    `json:"Head,omitempty"`
	SubTags  []*Tag  `json:"SubTags,omitempty"`
	Children []*Tags `json:"Children,omitempty"`
}

// docLeaves returns Head+SubTags when this Tags is a pure-text document (no Children, every leaf
// carrying only a distinct ContentType + Text), else ok=false.
func (tagsValue *Tags) docLeaves() (leaves []*Tag, ok bool) {
	if len(tagsValue.Children) > 0 {
		return nil, false
	}
	if tagsValue.Head != nil {
		leaves = append(leaves, tagsValue.Head)
	}
	leaves = append(leaves, tagsValue.SubTags...)
	seen := make(map[string]bool, len(leaves))
	for _, leaf := range leaves {
		pureText := leaf != nil && leaf.ContentType != "" && leaf.URI == "" &&
			leaf.UID_0 == 0 && leaf.UID_1 == 0 &&
			leaf.I == 0 && leaf.J == 0 && leaf.K == 0 && leaf.Units == 0
		if !pureText || seen[leaf.ContentType] {
			return nil, false
		}
		seen[leaf.ContentType] = true
	}
	return leaves, len(leaves) > 0
}

func (tagsValue *Tags) MarshalJSON() ([]byte, error) {
	if leaves, ok := tagsValue.docLeaves(); ok {
		doc := make(map[string]string, len(leaves))
		for _, leaf := range leaves {
			doc[leaf.ContentType] = leaf.Text
		}
		return json.Marshal(doc)
	}
	return json.Marshal(tagsTreeJSON{Head: tagsValue.Head, SubTags: tagsValue.SubTags, Children: tagsValue.Children})
}

func (tagsValue *Tags) UnmarshalJSON(data []byte) error {
	probe := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &probe); err != nil {
		return err
	}
	_, hasHead := probe["Head"]
	_, hasSub := probe["SubTags"]
	_, hasKids := probe["Children"]
	if hasHead || hasSub || hasKids {
		wire := tagsTreeJSON{}
		if err := json.Unmarshal(data, &wire); err != nil {
			return err
		}
		tagsValue.Head, tagsValue.SubTags, tagsValue.Children = wire.Head, wire.SubTags, wire.Children
		return nil
	}
	// ContentType → Text document.  Head/SubTag placement is immaterial to ByContentType (the
	// sole reader), so emit all as SubTags in sorted ContentType order for a deterministic value.
	doc := map[string]string{}
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}
	cts := make([]string, 0, len(doc))
	for contentType := range doc {
		cts = append(cts, contentType)
	}
	sort.Strings(cts)
	tagsValue.Head, tagsValue.Children = nil, nil
	tagsValue.SubTags = make([]*Tag, 0, len(cts))
	for _, contentType := range cts {
		tagsValue.SubTags = append(tagsValue.SubTags, &Tag{ContentType: contentType, Text: doc[contentType]})
	}
	return nil
}

// --- amp.CrateRef ---------------------------------------------------------

type crateRefJSON struct {
	CrateURI string `json:"CrateURI,omitempty"`
	BlobID   string `json:"BlobID,omitempty"`
}

func (crate *CrateRef) MarshalJSON() ([]byte, error) {
	return json.Marshal(crateRefJSON{
		CrateURI: crate.CrateURI,
		BlobID:   uidToBase32(crate.BlobID_0, crate.BlobID_1),
	})
}

func (crate *CrateRef) UnmarshalJSON(data []byte) error {
	wire := crateRefJSON{}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	blobID, err := uidFromBase32(wire.BlobID)
	if err != nil {
		return err
	}
	crate.CrateURI = wire.CrateURI
	crate.BlobID_0, crate.BlobID_1 = blobID[0], blobID[1]
	return nil
}

// --- amp.Address ----------------------------------------------------------
//
// An Address is up to five UIDs (PlanetID, NodeID, AttrID, ItemID, EditID).
// On the JSON wire it is a single base32 string over the concatenated
// big-endian UID bytes — one decode, no delimiters — rather than up to five
// separate fields.  The component count is implied by length:
//
//	3 UIDs (48 bytes) →           NodeID, AttrID, ItemID           (ElementID)
//	4 UIDs (64 bytes) →           NodeID, AttrID, ItemID, EditID
//	5 UIDs (80 bytes) → PlanetID, NodeID, AttrID, ItemID, EditID
//
// The minimal form is emitted: PlanetID present ⇒ 5; else EditID present ⇒ 4;
// else 3.

func (addr *Address) MarshalJSON() ([]byte, error) {
	planet := tag.UID{addr.PlanetID_0, addr.PlanetID_1}
	node := tag.UID{addr.NodeID_0, addr.NodeID_1}
	attr := tag.UID{addr.AttrID_0, addr.AttrID_1}
	item := tag.UID{addr.ItemID_0, addr.ItemID_1}
	edit := tag.UID{addr.EditID_0, addr.EditID_1}

	if planet.IsNil() && node.IsNil() && attr.IsNil() && item.IsNil() && edit.IsNil() {
		return json.Marshal("")
	}

	uids := []tag.UID{node, attr, item}
	switch {
	case !planet.IsNil():
		uids = []tag.UID{planet, node, attr, item, edit}
	case !edit.IsNil():
		uids = []tag.UID{node, attr, item, edit}
	}

	buf := make([]byte, len(uids)*tag.UID_Size)
	for i, uid := range uids {
		binary.BigEndian.PutUint64(buf[i*tag.UID_Size:], uid[0])
		binary.BigEndian.PutUint64(buf[i*tag.UID_Size+8:], uid[1])
	}
	return json.Marshal(encode.Base32Encoding.EncodeToString(buf))
}

func (addr *Address) UnmarshalJSON(data []byte) error {
	str := ""
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	*addr = Address{}
	if str == "" {
		return nil
	}
	buf, err := encode.Base32Encoding.DecodeString(str)
	if err != nil {
		return err
	}

	uids := make([]tag.UID, len(buf)/tag.UID_Size)
	if len(uids)*tag.UID_Size != len(buf) || len(uids) < 3 || len(uids) > 5 {
		return fmt.Errorf("amp.Address: expected 3-5 packed UIDs, got %d bytes", len(buf))
	}
	for i := range uids {
		uids[i] = tag.UID{
			binary.BigEndian.Uint64(buf[i*tag.UID_Size:]),
			binary.BigEndian.Uint64(buf[i*tag.UID_Size+8:]),
		}
	}

	cursor := 0
	if len(uids) == 5 {
		addr.PlanetID_0, addr.PlanetID_1 = uids[0][0], uids[0][1]
		cursor = 1
	}
	addr.NodeID_0, addr.NodeID_1 = uids[cursor][0], uids[cursor][1]
	addr.AttrID_0, addr.AttrID_1 = uids[cursor+1][0], uids[cursor+1][1]
	addr.ItemID_0, addr.ItemID_1 = uids[cursor+2][0], uids[cursor+2][1]
	if len(uids) >= 4 {
		addr.EditID_0, addr.EditID_1 = uids[cursor+3][0], uids[cursor+3][1]
	}
	return nil
}
