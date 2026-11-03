// Package data is a grab-bag of small data helpers: streamable Asset publishing,
// media-recording segmentation, process-seeded hashing, a []byte-keyed map, and
// proto / zig-zag encoding utilities.
package data

import (
	"io"
	"time"

	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"github.com/art-media-platform/amp.SDK/stdlib/task"
)

// PublishOp names the outward effect a PublishAsset call performs; the
// publisher judges Source.Actor's standing by it (SD-invite-governance §5).
// PublishOpUnset is refused at every publisher: a caller names its effect.
type PublishOp int32

const (
	PublishOpUnset        PublishOp = 0
	PublishNewExposure    PublishOp = 1 // a new URL exposing Source.Planet's content: Actor must stand JoinConfirmed there
	PublishAuthorizedRead PublishOp = 2 // a read the caller already authorized: a non-nil Gate, or a registered public planet
	PublishLocalDelivery  PublishOp = 3 // the session's own local capability: Actor is the login member, no planet
)

// PublishSource is the actor and source context a publisher judges — the
// planet the asset exposes (nil for PublishLocalDelivery), the member
// performing the effect, and the effect itself.  In-process, never wire.
type PublishSource struct {
	Planet tag.UID
	Actor  tag.UID
	Op     PublishOp
}

// PublishOpts specifies options when publishing an asset.
type PublishOpts struct {
	AssetID   string        // URL path element for this asset; if empty, a unique ID is generated. If an asset with this ID already exists, the entry is updated.
	Expiry    time.Duration // If <= 0, the publisher chooses the expiration period
	HostAddr  string        // Domain or IP address used in the generated URL; if empty -> "localhost"
	OnExpired func()        // Called when the asset expires
	Gate      any           // Publisher-defined admission scope: the URL admits only a request the publisher verifies against it (a media token); nil = the URL serves any GET. A publisher that cannot honor a non-nil Gate MUST refuse (status.Code_Unsupported) — never publish it ungated.
	// Source: the effect and its actor (SD-invite-governance §5) — a publisher
	// judges the actor's standing per Op and refuses PublishOpUnset, as each
	// publisher adopts the contract.
	Source PublishSource
}

// Publisher publishes an Asset to a URL.
// If opts.AssetID is set and an asset with that ID already exists, the entry is updated.
// If opts.AssetID is empty, a unique ID is generated for the URL path.
type Publisher interface {
	PublishAsset(asset Asset, opts PublishOpts) (URL string, err error)
}

// Asset is a flexible wrapper for any data asset that can be streamed (audio, video, files, etc).
type Asset interface {

	// Short name or description of this asset used for logging / debugging.
	Label() string

	// Returns the media (MIME) type of the asset.
	ContentType() string

	// OnStart is called when this asset is live within the given context.
	// An Asset should call ctx.Close() if a fatal error occurs or its underlying asset becomes unavailable.
	OnStart(ctx task.Context) error

	// Called when this asset is requested by a client for read access.
	NewAssetReader() (AssetReader, error)
}

// AssetReader provides read and seek access to its parent Asset.
//
// Close() is called when:
//   - the AssetReader is no longer needed (called externally), or
//   - the AssetReader's parent Asset becomes unavailable.
//
// Close() could be called at any time from a goroutine outside of a Read() or Seek() call.
type AssetReader interface {
	io.ReadSeekCloser
}
