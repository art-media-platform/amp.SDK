package data

import (
	"time"

	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// Phase is the externally-reported state of a recording engine, surfaced to a
// client through a Sink so a UI can show capture progress.
type Phase int

const (
	Connecting Phase = iota // dialing / negotiating the source
	Listening               // connected, waiting for the next segment boundary
	Recording               // actively writing a segment
	Committing              // sealing / persisting a completed segment
	Done                    // source ended cleanly
	Failed                  // source errored out
)

// Segment is one completed unit of recorded media handed from a recording engine
// to a Sink.  Type parameter M is engine-specific metadata the Sink uses to label,
// index, and address the segment (e.g. parsed track tags); a Sink that needs none
// uses struct{}.
//
// Source is a seekable reader over the captured bytes; the Sink owns it after
// Submit and must Close it, which releases the backing store (removing a temp file
// or freeing an in-memory buffer).
//
// Time is held as the exact integer rational TickDelta / TickRate (the QuickTime
// time model) rather than as floating-point seconds, so summing many fixed-rate
// segments never drifts and consecutive segments tile a stream exactly:
// segment N's TickOffset + TickDelta == segment N+1's TickOffset.  For a byte-rate
// stream the natural tick is the byte (TickRate is the byte rate); for sampled
// media the tick is one sample at TickRate Hz.
//
// TimeID is the segment's time-ordered capture-start ID (tag.NowID): a live sink
// commits under it so segments tail in capture order, while an archive sink keeps
// it as capture-time metadata and commits under a content key it derives from Meta.
type Segment[M any] struct {
	Source     AssetReader // captured bytes; Sink-owned after Submit, Close releases the backing store
	Bytes      int64       // storage byte length of Source
	TickOffset int64       // cumulative tick offset of this segment's start on the stream timeline
	TickDelta  int64       // this segment's duration as an exact tick span (numerator)
	TickRate   int64       // ticks per second — the timescale / denominator (0 when the source rate is unknown)
	Format     string      // content (MIME) type
	TimeID     tag.UID     // time-ordered capture-start ID (tag.NowID)
	Meta       M           // engine-specific labels / metadata
}

// Duration returns the segment's playback duration (TickDelta / TickRate), or 0
// when the rate is unknown.  A float-seconds value, when one is needed, is just
// Duration().Seconds() — there is no stored seconds field to drift out of sync.
func (segment Segment[M]) Duration() time.Duration {
	return ticksToDuration(segment.TickDelta, segment.TickRate)
}

// StartTime returns the segment's start position on the stream timeline
// (TickOffset / TickRate), or 0 when the rate is unknown.
func (segment Segment[M]) StartTime() time.Duration {
	return ticksToDuration(segment.TickOffset, segment.TickRate)
}

// ticksToDuration converts an integer tick count over an integer timescale into a
// time.Duration, scaling the whole-second and sub-second parts separately so a
// long offset cannot overflow the intermediate multiply.
func ticksToDuration(ticks, rate int64) time.Duration {
	if rate <= 0 {
		return 0
	}
	whole := time.Duration(ticks/rate) * time.Second
	frac := time.Duration((ticks % rate) * int64(time.Second) / rate)
	return whole + frac
}

// Sink is a recording engine's output boundary: it persists completed Segments and
// reports status.  The engine knows nothing about storage, encryption, sync, or
// addressing; a production Sink seals blobs and commits CRDT records, while a test
// Sink records the calls.
type Sink[M any] interface {

	// Skip reports whether a segment with this metadata is already persisted, letting
	// the engine avoid spending IO to capture it.  The Sink derives its own dedup key
	// from meta; a live Sink returns false.
	Skip(meta M) bool

	// Submit hands a completed segment off for asynchronous persistence.  It is
	// non-blocking; the Sink takes ownership of segment.Source from here.
	Submit(segment *Segment[M])

	// Status reports a capture-phase transition for the client's status surface.
	Status(phase Phase, label string)
}

// SegmentState is the running state of an in-progress segment, evaluated by a
// BoundaryPolicy to decide where one segment ends and the next begins.  Signal
// carries an optional engine-specific event (e.g. a stream-metadata change) that
// an event-driven policy keys on; it is nil for threshold-driven policies.
type SegmentState struct {
	Bytes     int64
	TickDelta int64
	TickRate  int64
	Signal    any
}

// Duration returns the running segment's elapsed time (TickDelta / TickRate), or 0
// when the rate is unknown.
func (state SegmentState) Duration() time.Duration {
	return ticksToDuration(state.TickDelta, state.TickRate)
}

// SegmentInfo describes the segment that begins after a cut: its status label and
// the engine-specific metadata carried onto the Segment.  The recorder stamps the
// time-ordered TimeID itself, so a policy never mints identities.
type SegmentInfo[M any] struct {
	Label string // human label for status / logging
	Meta  M      // engine-specific metadata carried onto the Segment
}

// BoundaryPolicy decides where segments are cut.  The metadata-driven policy of a
// song recorder and the fixed-interval policy of a live segmenter are two
// implementations; one capture engine serves both by swapping the policy.
type BoundaryPolicy[M any] interface {

	// Cut reports whether the running segment should end now and, when it does,
	// describes the segment that begins next.
	Cut(state SegmentState) (cut bool, next SegmentInfo[M])
}

// IntervalPolicy cuts a segment every MaxDuration of elapsed time or MaxBytes of
// captured audio, whichever comes first.  It ignores SegmentState.Signal: cuts are
// driven purely by accumulated time and bytes, which is what a live segmenter wants.
// At least one limb must be set; a zero limb is disabled.
type IntervalPolicy[M any] struct {
	MaxDuration time.Duration
	MaxBytes    int64
	Label       string
}

// Cut implements BoundaryPolicy.
func (policy IntervalPolicy[M]) Cut(state SegmentState) (bool, SegmentInfo[M]) {
	overTime := policy.MaxDuration > 0 && state.Duration() >= policy.MaxDuration
	overBytes := policy.MaxBytes > 0 && state.Bytes >= policy.MaxBytes
	if !overTime && !overBytes {
		var none SegmentInfo[M]
		return false, none
	}
	var meta M
	return true, SegmentInfo[M]{Label: policy.Label, Meta: meta}
}
