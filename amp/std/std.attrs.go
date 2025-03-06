package std

import (
	"time"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

var (
	AppSpec     = tag.Expr{}.With("app")
	CellAttr    = tag.Expr{}.With("cell")
	SessionAttr = tag.Expr{}.With("session")
	//Channel     = tag.Expr{}.With("channel")

	AppState = AppSpec.With("state")

	LoginID           = RegisterAttr(SessionAttr, &amp.Login{}, "").ID
	LoginChallengeID  = RegisterAttr(SessionAttr, &amp.LoginChallenge{}, "").ID
	LoginResponseID   = RegisterAttr(SessionAttr, &amp.LoginResponse{}, "").ID
	LoginCheckpointID = RegisterAttr(SessionAttr, &amp.LoginCheckpoint{}, "").ID
	SessionErr        = RegisterAttr(SessionAttr, &amp.Err{}, "").ID
	PinRequestID      = RegisterAttr(SessionAttr, &amp.PinRequest{}, "").ID
	PinAdjustID       = RegisterAttr(SessionAttr, &amp.Tags{}, "PinAdjust").ID

	ClientAgentID = SessionAttr.With("ClientAgent").ID
	LaunchTag     = SessionAttr.With("launch.Tag")
	LaunchOAuth   = LaunchTag.With("oauth").ID

	CellChild = CellAttr.With("child.Tag.ID") // each TxOp.ItemID expresses a child cell ID

	CellTextTag    = CellAttr.With("text.Tag")
	CellLabel      = CellTextTag.With("label").ID
	CellCaption    = CellTextTag.With("caption").ID
	CellCollection = CellTextTag.With("collection").ID
	CellSynopsis   = CellTextTag.With("synopsis").ID

	CellContent = CellAttr.With("content")
	CellFSInfo  = CellContent.With("FSInfo").ID
	CellGlyphs  = CellContent.With("Tags.glyphs").ID
	CellLinks   = CellContent.With("Tags.links").ID
	CellMedia   = CellContent.With("Tag.media").ID
	CellVis     = CellContent.With("Tag.vis").ID

	// TileAttr denotes attributes of a cell's background tile, framing, and appearance (in contrast to the "content" of the cell)
	TileAttr   = CellAttr.With("tile")
	TileLayout = TileAttr.With("Tag.layout").ID
)

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

func (v *FSInfo) MarshalToStore(in []byte) (out []byte, err error) {
	return amp.MarshalPbToStore(v, in)
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
