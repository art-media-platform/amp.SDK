package std

import (
	"time"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

var (
	LoginSpec           = amp.SystemAttr.With("Login").ID
	LoginChallengeSpec  = amp.SystemAttr.With("LoginChallenge").ID
	LoginResponseSpec   = amp.SystemAttr.With("LoginResponse").ID
	LoginCheckpointSpec = amp.SystemAttr.With("LoginCheckpoint").ID

	Item000 = tag.Nil

	//PositionQRS  = amp.SystemAttr.With("position.QRS.mm").ID // https://www.redblobgames.com/grids/hexagons/#neighbors-cube
	LaunchURL    = amp.SystemAttr.With("LaunchURL").ID
	CellProperty = amp.SystemAttr.With("cell.property")

	CellChild   = CellProperty.With("child.Tag.ID") // each TxOp.ItemID expresses a child cell ID
	CellFSInfo  = CellProperty.With("FSInfo").ID
	CellContent = CellProperty.With("content")
	CellText    = CellProperty.With("text.Tag")

	CellLabel      = CellText.With("label").ID
	CellCaption    = CellText.With("caption").ID
	CellCollection = CellText.With("collection").ID
	CellSynopsis   = CellText.With("synopsis").ID

	CellGlyphs = CellContent.With("Tags.glyphs").ID
	CellLinks  = CellContent.With("Tags.links").ID
	CellMedia  = CellContent.With("Tag.media").ID
	CellVis    = CellContent.With("Tag.vis").ID

	// CellPropertyTagID = CellProperty.With("Tag.ID")
	// OrderByPlayID     = CellPropertyTagID.With("order-by.play").ID
	// OrderByTimeID     = CellPropertyTagID.With("order-by.time").ID
	// OrderByGeoID      = CellPropertyTagID.With("order-by.geo").ID
	// OrderByAreaID     = CellPropertyTagID.With("order-by.area").ID

)

const (
	// URL prefix for a glyph and is typically followed by a media (mime) type.
	GenericGlyphURL = "amp:glyph/"

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
		ID: &amp.Tag{
			URL: GenericGlyphURL + contentType,
		},
	}
}

func TagsForImageURL(imageURL string) *amp.Tags {
	return &amp.Tags{
		ID: &amp.Tag{
			URL:         imageURL,
			ContentType: GenericImageType,
		},
	}
}

func (v *Position) MarshalToStore(in []byte) (out []byte, err error) {
	return amp.MarshalPbToStore(v, in)
}

func (v *Position) TagExpr() tag.Expr {
	return amp.SystemAttr.With("Position")
}

func (v *Position) New() tag.Value {
	return &Position{}
}

func (v *FSInfo) MarshalToStore(in []byte) (out []byte, err error) {
	return amp.MarshalPbToStore(v, in)
}

func (v *FSInfo) TagExpr() tag.Expr {
	return amp.SystemAttr.With("FSInfo")
}

func (v *FSInfo) New() tag.Value {
	return &FSInfo{}
}

func (v *FSInfo) SetModifiedAt(t time.Time) {
	tag := tag.FromTime(t, false)
	v.ModifiedAt = int64(tag[0])
}

func (v *FSInfo) SetCreatedAt(t time.Time) {
	tag := tag.FromTime(t, false)
	v.CreatedAt = int64(tag[0])
}
