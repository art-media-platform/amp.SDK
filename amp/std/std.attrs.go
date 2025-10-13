package std

import (
	"path"
	"time"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

var (
	AppTag      = tag.Name{}.With("app")
	SessionAttr = tag.Name{}.With("session")
	ItemAttr    = tag.Name{}.With("item")

	AppState = AppTag.With("state")

	LoginID           = SessionAttr.With("Login").ID
	LoginChallengeID  = SessionAttr.With("LoginChallenge").ID
	LoginResponseID   = SessionAttr.With("LoginResponse").ID
	LoginCheckpointID = SessionAttr.With("LoginCheckpoint").ID
	SessionErrorID    = SessionAttr.With("Error").ID
	SessionTag        = SessionAttr.With("Tag")
	LaunchWeb         = SessionTag.With("www").ID
	LaunchOAuth       = SessionTag.With("oauth").ID

	ChildLink = ItemAttr.With("child.link.ID").ID // each TxOp.ItemID is an inline child item ID

	ItemTextTag    = ItemAttr.With("text.Tag")
	ItemLabel      = ItemTextTag.With("label").ID
	ItemCaption    = ItemTextTag.With("caption").ID
	ItemCollection = ItemTextTag.With("collection").ID
	ItemSynopsis   = ItemTextTag.With("synopsis").ID

	ItemFileInfo = ItemAttr.With("FileInfo").ID
	MainLink     = ItemAttr.With("Tag.link.main").ID
	MediaLink    = ItemAttr.With("Tag.link.media")

	ItemVis       = ItemAttr.With("Tag.vis").ID
	ItemBehaviors = ItemAttr.With("Tags.behaviors").ID
	ItemGlyphs    = ItemAttr.With("Tags.glyphs").ID
	ItemLinks     = ItemAttr.With("Tags.links").ID

	MediaItemID    = ItemAttr.With("av.MediaItem").ID
	MediaReleaseID = ItemAttr.With("av.Tag.MediaRelease").ID
)

const ()

const (
	DDC_MaxFraction = uint64(999999999 + 1) // 9 digits (000.123456789)
	DDC_Max         = float64(1000)
	DDC_to_Fixed    = float64(uint64(1)<<31) / DDC_Max
	// PublicTag_Category     = uint64(2541) << 32        // uint64(25.41 * DDC_to_Fixed) TODO
	// PublicTag_Category_DDC = PublicTag_Category + 1851 // Dewey Decimal Classification

	BulletSeparator = " · "
)

// Constructs a standard tag.UID expressing "{DDC_Whole}.{DDC_Decimal}"
func PublicTag_DDC(geoTile uint64, DDC_Whole, DDC_Decimal uint32) tag.UID {
	fract := (uint64(DDC_Decimal) << 32) / DDC_MaxFraction
	return tag.UID{
		geoTile,
		(uint64(DDC_Whole) << 32) | fract,
	}
}

const (
	// URL prefix for a glyph and is typically followed by a media (mime) type.
	ContentGlyphURI = "amp:glyph/"

	GenericImageType = "image/*"
	GenericAudioType = "audio/*"
	GenericVideoType = "video/*"
)

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
			URI:         imageURL,
			ContentType: GenericImageType,
		},
	}
}

func (v *FileInfo) MarshalToStore(in []byte) (out []byte, err error) {
	return amp.MarshalPbToStore(v, in)
}

func (v *FileInfo) New() amp.Value {
	return &FileInfo{}
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

func (v *GeoPath) MarshalToStore(dst []byte) ([]byte, error) {
	return amp.MarshalPbToStore(v, dst)
}

func (v *GeoPath) New() amp.Value {
	return &GeoPath{}
}
