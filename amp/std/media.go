package std

import (
	"fmt"
	"io"
	"sort"
	"sync"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/data"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"github.com/art-media-platform/amp.SDK/stdlib/task"
)

// A media stream presents the MediaSegment records on one (node, attr) cell as a
// single seekable byte stream — the channel's segments concatenated in capture
// order.  Any app that records media as time-ordered MediaSegments (a live radio
// segmenter, a podcast, a screen capture) turns its channel into one playable URL
// with these helpers; a URL-based player (Unity / web / a browser) opens it and
// scrubs via HTTP range requests.
//
// The reader is range-lazy: a segment's bytes are read from its durable Blob only
// when a Read/Seek touches that segment, and at most MaxResidentBytes of segment
// data stays open at once.  A four-hour mix is therefore served without ever
// holding the whole recording in memory — only a compact offset index plus the
// current read window.  The inline MediaSegment.Payload is intentionally unused
// here (it is the live-edge / integrity copy); the stream serves the Blob.

const (
	// DefaultMediaResidentBytes bounds the segment bytes a media stream keeps open
	// while serving when MediaStreamOpts.MaxResidentBytes is unset.  As a byte
	// budget it adapts to the medium on its own: a 128 kbps audio segment is ~96 KB
	// so the window spans dozens of segments, while a single 4K-video segment can
	// fill it — the count flexes, the resident bytes stay bounded.
	DefaultMediaResidentBytes = 8 << 20

	// minMediaResidentBytes is the floor for an explicit budget; a reader always
	// keeps at least the one segment it is currently reading, even past this.
	minMediaResidentBytes = 1 << 20
)

// MediaStreamSource names the (planet, node, attr) cell whose MediaSegment items
// are streamed as one continuous asset.
type MediaStreamSource struct {
	Planet tag.UID
	Node   tag.UID
	Attr   tag.UID
}

// MediaStreamOpts tunes how the source attr is served.
type MediaStreamOpts struct {
	// MaxResidentBytes bounds the segment bytes held open while serving — the
	// buffer window.  Older segments are evicted once the budget is exceeded, so a
	// long recording is range-loaded a window at a time and never fully resides in
	// heap.  Zero selects DefaultMediaResidentBytes.  The floor is always the one
	// segment currently being read.
	MaxResidentBytes int64

	// ContentType is the codec/MIME hint for the served stream (e.g. "audio/mpeg").
	// When empty it is derived from the first segment's Format.  Supplying it lets a
	// live stream publish a playable URL before its first segment commits.
	ContentType string
}

// BlobOpener resolves a BlobRef to a seekable plaintext reader — retrieve →
// decrypt-if-sealed → validate → cache.  amp.Session satisfies it via OpenBlob, so
// a media stream reads through the one decrypt-and-cache site and holds no key
// material.  It is the single dependency the lazy reader needs, which also keeps
// the reader unit-testable with a counting fake.
type BlobOpener interface {
	OpenBlob(planetID tag.UID, ref *amp.BlobRef) (data.AssetReader, error)
}

// MediaStreamAttr presents the source attr as one seekable data.Asset.  The
// segment index is (re)built per reader from a snapshot of the attr, so a growing
// live channel is reflected on each request; the bytes are range-loaded lazily
// from each segment's Blob.
func MediaStreamAttr(appCtx amp.AppContext, src MediaStreamSource, opts MediaStreamOpts) (data.Asset, error) {
	if appCtx == nil {
		return nil, fmt.Errorf("std: MediaStreamAttr requires an AppContext")
	}
	if src.Node.IsNil() || src.Attr.IsNil() {
		return nil, fmt.Errorf("std: MediaStreamAttr requires a node and attr")
	}
	residency := opts.MaxResidentBytes
	if residency <= 0 {
		residency = DefaultMediaResidentBytes
	} else if residency < minMediaResidentBytes {
		residency = minMediaResidentBytes
	}
	return &mediaStreamAsset{
		appCtx:      appCtx,
		src:         src,
		residency:   residency,
		contentType: opts.ContentType,
	}, nil
}

// PublishMediaStream builds MediaStreamAttr and publishes it through the session's
// data.Publisher (the host web service), returning a MediaLink Tag{URI, ContentType}
// ready to Upsert at std.Attr.MediaLink — the playable URL a player opens (scrub =
// HTTP range over the lazy reader).
func PublishMediaStream(appCtx amp.AppContext, src MediaStreamSource, opts MediaStreamOpts, pub data.PublishOpts) (*amp.Tag, error) {
	asset, err := MediaStreamAttr(appCtx, src, opts)
	if err != nil {
		return nil, err
	}
	publisher := appCtx.Session().AssetPublisher()
	if publisher == nil {
		return nil, fmt.Errorf("std: PublishMediaStream: session has no asset publisher")
	}
	url, err := publisher.PublishAsset(asset, pub)
	if err != nil {
		return nil, err
	}
	return &amp.Tag{URI: url, ContentType: asset.ContentType()}, nil
}

// segEntry is one segment's place on the byte timeline and its durable blob.  The
// inline Payload is deliberately absent: the index stays compact (a few dozen bytes
// per segment) however long the recording runs.
type segEntry struct {
	offset int64
	length int64
	blob   *amp.BlobRef
}

type mediaStreamAsset struct {
	appCtx    amp.AppContext
	src       MediaStreamSource
	residency int64

	mu          sync.Mutex
	index       []segEntry
	size        int64
	contentType string
}

func (a *mediaStreamAsset) Label() string                  { return "media:" + a.src.Node.Base32() }
func (a *mediaStreamAsset) OnStart(ctx task.Context) error { return nil }

func (a *mediaStreamAsset) ContentType() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.contentType == "" {
		a.loadLocked() // derive the codec from the first segment when no hint was given
	}
	return a.contentType
}

// NewAssetReader rebuilds the index from a fresh snapshot (so a growing live attr
// is reflected) and returns a range-lazy reader over the segment blobs.
func (a *mediaStreamAsset) NewAssetReader() (data.AssetReader, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.loadLocked(); err != nil {
		return nil, err
	}
	if len(a.index) == 0 {
		return nil, fmt.Errorf("std: media stream %s has no segments yet", a.src.Node.Base32())
	}
	return newMediaStreamReader(a.appCtx.Session(), a.src.Planet, a.index, a.size, a.residency), nil
}

// loadLocked scans the attr, orders the segments by capture-time ItemID, and lays
// them out on a contiguous byte timeline.  Caller holds a.mu.
func (a *mediaStreamAsset) loadLocked() error {
	coll := &segCollector{revision: tag.NowID(), attr: a.src.Attr}
	if err := LoadItems(a.appCtx, a.src.Node, a.src.Attr, coll); err != nil {
		return err
	}
	sort.Slice(coll.raw, func(i, j int) bool {
		return coll.raw[i].itemID.CompareTo(coll.raw[j].itemID) < 0
	})
	index := make([]segEntry, len(coll.raw))
	offset := int64(0)
	for i, rawSeg := range coll.raw {
		index[i] = segEntry{offset: offset, length: rawSeg.length, blob: rawSeg.blob}
		offset += rawSeg.length
	}
	a.index = index
	a.size = offset
	if a.contentType == "" {
		a.contentType = coll.contentType
	}
	return nil
}

// segRaw is one collected segment before it is ordered and laid out.
type segRaw struct {
	itemID tag.UID
	length int64
	blob   *amp.BlobRef
}

// segCollector gathers a node's MediaSegment items under one attr — their blob,
// byte length, and ItemID — dropping the inline Payload.
type segCollector struct {
	revision    tag.UID
	attr        tag.UID
	raw         []segRaw
	contentType string
}

func (c *segCollector) Revision() tag.UID { return c.revision }

func (c *segCollector) OnNodeUpdate(update amp.NodeUpdate) {
	if update.Tx == nil {
		return
	}
	for _, op := range update.Tx.Ops {
		if op.Addr.AttrID != c.attr {
			continue
		}
		seg := &MediaSegment{}
		if err := update.Tx.ExtractValue(c.attr, op.Addr.ItemID, seg); err != nil {
			continue
		}
		if seg.Blob == nil {
			continue // no durable bytes to serve
		}
		length := seg.TickDelta
		if seg.Blob.AssetTag != nil && seg.Blob.AssetTag.I > 0 {
			length = seg.Blob.AssetTag.I // the blob's byte size — robust across MediaSegment.Units
		}
		if length <= 0 {
			continue
		}
		c.raw = append(c.raw, segRaw{itemID: op.Addr.ItemID, length: length, blob: seg.Blob})
		if c.contentType == "" {
			if seg.Format != "" {
				c.contentType = seg.Format
			} else if seg.Blob.AssetTag != nil {
				c.contentType = seg.Blob.AssetTag.ContentType
			}
		}
	}
}

// mediaStreamReader is an io.ReadSeekCloser over the concatenated segment blobs.
// It opens only the segment a Read touches and keeps at most residency bytes of
// segments open (LRU), so serving any length of stream is bounded-heap.
type mediaStreamReader struct {
	opener    BlobOpener
	planet    tag.UID
	index     []segEntry
	size      int64
	residency int64

	pos    int64
	open   []*openSeg // LRU order, most-recently-used at the tail
	openSz int64
	closed bool
}

type openSeg struct {
	seg    int
	length int64
	reader data.AssetReader
}

func newMediaStreamReader(opener BlobOpener, planet tag.UID, index []segEntry, size, residency int64) *mediaStreamReader {
	return &mediaStreamReader{opener: opener, planet: planet, index: index, size: size, residency: residency}
}

func (r *mediaStreamReader) Read(dst []byte) (int, error) {
	if r.closed {
		return 0, fmt.Errorf("std: media stream reader closed")
	}
	if r.pos >= r.size {
		return 0, io.EOF
	}
	seg := r.segmentAt(r.pos)
	if seg < 0 {
		return 0, io.EOF
	}
	entry := r.index[seg]
	held, err := r.acquire(seg)
	if err != nil {
		return 0, err
	}
	intra := r.pos - entry.offset
	if _, err := held.reader.Seek(intra, io.SeekStart); err != nil {
		return 0, err
	}
	if remain := entry.length - intra; int64(len(dst)) > remain {
		dst = dst[:remain] // never read past this segment in one call
	}
	read, err := held.reader.Read(dst)
	r.pos += int64(read)
	if err == io.EOF && r.pos < r.size {
		err = nil // the stream continues into the next segment
	}
	return read, err
}

func (r *mediaStreamReader) Seek(offset int64, whence int) (int64, error) {
	abs := int64(0)
	switch whence {
	case io.SeekStart:
		abs = offset
	case io.SeekCurrent:
		abs = r.pos + offset
	case io.SeekEnd:
		abs = r.size + offset
	default:
		return 0, fmt.Errorf("std: media stream invalid whence %d", whence)
	}
	if abs < 0 {
		return 0, fmt.Errorf("std: media stream negative position %d", abs)
	}
	r.pos = abs // lazy: the covering segment is opened on the next Read
	return abs, nil
}

func (r *mediaStreamReader) Close() error {
	r.closed = true
	firstErr := error(nil)
	for _, held := range r.open {
		if err := held.reader.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	r.open = nil
	r.openSz = 0
	return firstErr
}

// segmentAt returns the index of the segment covering byte pos, or -1.
func (r *mediaStreamReader) segmentAt(pos int64) int {
	seg := sort.Search(len(r.index), func(k int) bool { return r.index[k].offset > pos }) - 1
	if seg < 0 {
		return -1
	}
	entry := r.index[seg]
	if pos < entry.offset || pos >= entry.offset+entry.length {
		return -1
	}
	return seg
}

// acquire returns an open reader for segment seg, opening its blob if needed and
// evicting least-recently-used segments back within the resident budget.
func (r *mediaStreamReader) acquire(seg int) (*openSeg, error) {
	last := len(r.open) - 1
	for k := last; k >= 0; k-- {
		held := r.open[k]
		if held.seg != seg {
			continue
		}
		if k != last { // promote to most-recently-used
			copy(r.open[k:], r.open[k+1:])
			r.open[last] = held
		}
		return held, nil
	}
	reader, err := r.opener.OpenBlob(r.planet, r.index[seg].blob)
	if err != nil {
		return nil, err
	}
	held := &openSeg{seg: seg, length: r.index[seg].length, reader: reader}
	r.open = append(r.open, held)
	r.openSz += held.length
	r.evict()
	return held, nil
}

// evict closes least-recently-used segments while over budget, always keeping the
// most-recently-used one (the segment currently being read).
func (r *mediaStreamReader) evict() {
	for r.openSz > r.residency && len(r.open) > 1 {
		victim := r.open[0]
		r.open = r.open[1:]
		r.openSz -= victim.length
		victim.reader.Close()
	}
}
