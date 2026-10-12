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

// PublicTag_DDC constructs a standard tag.UID expressing "{DDC_Whole}.{DDC_Decimal}"
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
			URI:            ContentGlyphURI + contentType,
			ContentTypeRaw: contentType, // the leaf declares its own type — readers use Tag.ContentType(), never the URI suffix
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
// (AOM SD-channel-governance.md §6).  Web-bridge member writes and codex content
// imports both reject these.  LawWithdraw is intentionally excluded: a member may
// withdraw their own content.
func IsGovernanceLawAttr(attrID tag.UID) bool {
	return attrID == Attr.LawChannelEpoch.ID ||
		attrID == Attr.LawMemberEpoch.ID ||
		attrID == Attr.LawPlanetEpoch.ID ||
		attrID == Attr.LawPlanetOrigin.ID ||
		attrID == Attr.LawEquivalence.ID
}

// IsPlanetPublicAttr reports whether attrID stays planet-public on a
// Confidential planet because a node holding no planet key must read it
// (AOM SD-security-sync.md §7.7).  A closed set, so every binary of one
// release decides alike: every other attr is content, sealed by default.
func IsPlanetPublicAttr(attrID tag.UID) bool {
	if IsGovernanceLawAttr(attrID) {
		return true
	}
	switch attrID {
	case Attr.LawEpochLink.ID,
		Attr.Brand.ID,
		Attr.PlanetInvitePolicy.ID,
		Attr.PlanetInviteRedemption.ID,
		Attr.NameServiceRecord.ID,
		Attr.FederationDirectory.ID,
		Attr.PlanetStorageOpts.ID,
		Attr.NodeLogEntry.ID,
		Attr.NodeMessage.ID,
		Attr.LedgerAttestation.ID:
		return true
	}
	return false
}

// SealDomain classifies a tx's ops by IsPlanetPublicAttr (TxSealDomain).
type SealDomain int32

const (
	SealDomain_Empty        SealDomain = iota // no ops
	SealDomain_PlanetPublic                   // every op is planet-public
	SealDomain_Content                        // every op is content
	SealDomain_Mixed                          // planet-public and content ops
)

// TxSealDomain reports the seal domain of tx's ops.  One tx is one encryption
// context, so a host refuses a Mixed tx on a Confidential planet (AOM
// ZO-design-conventions.md §3.3).
func TxSealDomain(tx *amp.TxMsg) SealDomain {
	planetPublic := false
	content := false
	for _, op := range tx.Ops {
		if IsPlanetPublicAttr(op.Addr.AttrID) {
			planetPublic = true
		} else {
			content = true
		}
	}
	switch {
	case planetPublic && content:
		return SealDomain_Mixed
	case content:
		return SealDomain_Content
	case planetPublic:
		return SealDomain_PlanetPublic
	}
	return SealDomain_Empty
}

func (domain SealDomain) String() string {
	switch domain {
	case SealDomain_PlanetPublic:
		return "planet-public"
	case SealDomain_Content:
		return "content"
	case SealDomain_Mixed:
		return "mixed"
	}
	return "empty"
}
