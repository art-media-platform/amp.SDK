package data

import (
	"io"
	"time"

	"github.com/art-media-platform/amp.SDK/stdlib/task"
)

// PublishOpts specifies options when publishing an asset.
type PublishOpts struct {
	AssetID   string        // URL path element for this asset; if empty, a unique ID is generated. If an asset with this ID already exists, the entry is updated.
	Expiry    time.Duration // If <= 0, the publisher chooses the expiration period
	HostAddr  string        // Domain or IP address used in the generated URL; if empty -> "localhost"
	OnExpired func()        // Called when the asset expires
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
