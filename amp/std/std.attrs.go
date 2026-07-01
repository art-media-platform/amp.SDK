package std

import (
	"path"
	"time"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

const (
	DDC_MaxFraction = uint64(999999999 + 1) // 9 digits (000.123456789)
	DDC_Max         = float64(1000)
	DDC_to_Fixed    = float64(uint64(1)<<31) / DDC_Max
)

// Constructs a standard tag.UID expressing "{DDC_Whole}.{DDC_Decimal}"
func PublicTag_DDC(geoTile uint64, DDC_Whole, DDC_Decimal uint32) tag.UID {
	fract := (uint64(DDC_Decimal) << 32) / DDC_MaxFraction
	return tag.UID{
		geoTile,
		(uint64(DDC_Whole) << 32) | fract,
	}
}

// Common universal glyphs
var (
	GenericFolderTags = TagsForContentType("application/x-directory")
)

func TagsForContentType(contentType string) *amp.Tags {
	return &amp.Tags{
		Head: &amp.Tag{
			URI: ContentGlyphURI + contentType,
		},
	}
}

func TagsForImageURL(imageURL string) *amp.Tags {
	return &amp.Tags{
		Head: &amp.Tag{
			URI:            imageURL,
			ContentTypeRaw: GenericImageType,
		},
	}
}

func (v *FileInfo) Pathname() string {
	return path.Join(v.DirName, v.ItemName)
}

func (v *FileInfo) SetModifiedAt(t time.Time) {
	uid := tag.UID_FromTime(t)
	v.ModifiedAt = int64(uid[0])
}

func (v *FileInfo) SetCreatedAt(t time.Time) {
	uid := tag.UID_FromTime(t)
	v.CreatedAt = int64(uid[0])
}

// IsGovernanceLawAttr reports whether attrID is a governance-law attr — the
// channel/member/planet authority records that flow only through founder/admin
// paths, never a member content write or a content-bearing artifact stream
// (SD-channel-governance.md §6).  Web-bridge member writes and codex content
// imports both reject these.  LawWithdraw is intentionally excluded: a member may
// withdraw their own content.
func IsGovernanceLawAttr(attrID tag.UID) bool {
	return attrID == Attr.LawChannelEpoch.ID ||
		attrID == Attr.LawMemberEpoch.ID ||
		attrID == Attr.LawPlanetEpoch.ID ||
		attrID == Attr.LawPlanetOrigin.ID ||
		attrID == Attr.LawEquivalence.ID
}
