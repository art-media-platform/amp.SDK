package media

import (
	"io"
	"time"

	"github.com/art-media-platform/amp.SDK/stdlib/task"
)

// Options when publishing an asset
type PublishOpts struct {
	Expiry    time.Duration // If <= 0, the publisher chooses the expiration period
	HostAddr  string        // Domain or IP address used in the generated URL; if empty -> "localhost"
	OnExpired func()        // Called when the asset expires
}

// Publishes a media.Asset to a randomly generated URL until the idle expiration is reached.
// If idleExpiry == 0, the publisher will choose an expiration period.
type Publisher interface {
	PublishAsset(asset Asset, opts PublishOpts) (URL string, err error)
}

// Asset is a flexible wrapper for any data asset that can be streamed -- often audio or video.
type Asset interface {

	// Short name or description of this asset used for logging / debugging.
	Label() string

	// Returns the media (MIME) type of the asset.
	ContentType() string

	// OnStart is called when this asset is live within the given context.
	// An Asset should call ctx.Close() if a fatal error occurs or its underlying asset becomes unavailable.
	OnStart(ctx task.Context) error

	// Called when this asset is requested by a client for read access
	NewAssetReader() (AssetReader, error)
}

// AssetReader provides read and seek access to its parent media.Asset.
//
// Close() is called when:
//   - the AssetReader is no longer needed (called externally), or
//   - the AssetReader's parent media.Asset becomes unavailable.
//
// Close() could be called at any time from a goroutine outside of a Read() or Seek() call.
type AssetReader interface {
	io.ReadSeekCloser
}
