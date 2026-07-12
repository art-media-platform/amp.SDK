package std

import (
	"fmt"
	"strings"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// A playlist is the ordered-mutable-sequence design (AD-playlists) applied to
// media: rows are item.MediaEntry cells keyed by a minted entryID on the
// playlist node, position is the independent item.MediaRank cell on the same
// entryID, and render order is (Rank, entryID) — a row with no rank orders by
// its entryID.  Entries are a bag: duplicates are legal, and the same track
// may appear under two entryIDs.  Tracks are separate nodes; MediaEntry.Ref
// points at one (or directly at a blob asset UID or a URI, in which case the
// entry's own fields are the only metadata).

// WriteMediaEntry upserts one playlist row at entryID on listNode.
func WriteMediaEntry(tx *amp.TxMsg, listNode, entryID tag.UID, entry *MediaEntry) error {
	if entry == nil {
		return fmt.Errorf("std: WriteMediaEntry requires a MediaEntry")
	}
	return tx.Upsert(listNode, Attr.MediaEntry.ID, entryID, entry)
}

// WriteMediaRank upserts a row's position — the 16-byte cell a reorder costs.
func WriteMediaRank(tx *amp.TxMsg, listNode, entryID, rank tag.UID) error {
	return tx.Upsert(listNode, Attr.MediaRank.ID, entryID, MediaRankOf(rank))
}

// AppendMediaEntry appends ONE interactively added row: it mints the entryID
// and writes no rank, so the row orders by its mint time.  A programmatic
// multi-row add must use AppendMediaEntries instead — inside one clock tick
// NowID order is entropy, not sequence (AD-playlists §3).
func AppendMediaEntry(tx *amp.TxMsg, listNode tag.UID, entry *MediaEntry) (tag.UID, error) {
	entryID := tag.NowID()
	return entryID, WriteMediaEntry(tx, listNode, entryID, entry)
}

// AppendMediaEntries appends rows in bulk — an album import, a provider
// mirror — partitioning the rank space after afterRank (the list's last rank,
// or a zero UID onto an empty or unranked tail) in one RanksAcross call, so
// the rows land in slice order regardless of how many entryIDs share a clock
// tick.  Returns the minted entryIDs in row order.
func AppendMediaEntries(tx *amp.TxMsg, listNode, afterRank tag.UID, entries []*MediaEntry) ([]tag.UID, error) {
	ranks, ok := tag.RanksAcross(afterRank, tag.RankCeil, len(entries))
	if !ok {
		return nil, fmt.Errorf("std: AppendMediaEntries: no rank space after %v for %d rows", afterRank, len(entries))
	}
	entryIDs := make([]tag.UID, len(entries))
	for i, entry := range entries {
		entryID := tag.NowID()
		if err := WriteMediaEntry(tx, listNode, entryID, entry); err != nil {
			return nil, err
		}
		if err := WriteMediaRank(tx, listNode, entryID, ranks[i]); err != nil {
			return nil, err
		}
		entryIDs[i] = entryID
	}
	return entryIDs, nil
}

// RankUID returns the rank as its tag.UID order key; nil-safe (nil => zero
// UID, which orders the row by entryID).
func (m *MediaRank) RankUID() tag.UID {
	if m == nil {
		return tag.UID{}
	}
	return tag.UID{m.Rank_0, m.Rank_1}
}

// MediaRankOf wraps a rank UID as the MediaRank cell value.
func MediaRankOf(rank tag.UID) *MediaRank {
	return &MediaRank{
		Rank_0: rank[0],
		Rank_1: rank[1],
	}
}

// A track's item.media.sources.Tags is the set of ways to get the bytes, one
// Tag per source (AD-playlists §5).  The three source kinds are peers,
// distinguished only by which field carries the pointer:
//
//   - UID set, no URI  — a content-addressed blob: resolve amp.blob.ref[UID]
//     on the same node, then Session.OpenBlob.  Hash-verified, epoch-decrypted,
//     seekable, offline-capable.
//   - URI https:/file: — open directly.
//   - URI custom scheme (spotify:, ...) — the app owning the scheme resolves
//     it at play time; the resolved stream is short-lived by design and is
//     never stored.
//
// ContentTypeRaw carries the codec; I + Units_BitrateKbps carry the quality —
// the same leaf grammar as item.glyphs.Tags and av.StationProfile.Tiers.

// SourceCriteria narrows SelectBestSource.
type SourceCriteria struct {
	ContentType string // "" = any; else a prefix match ("audio/" or exact "audio/mpeg")
	MaxKbps     int64  // 0 = unlimited; caps the preferred bitrate
	Offline     bool   // true = only blob sources qualify
}

// SelectBestSource picks the best source leaf from a media sources bag, or
// nil when none qualifies: filter by criteria, prefer the highest bitrate
// within MaxKbps, then the lowest above it, then bitrate-untagged; ties break
// blob over direct URL over custom scheme.
func SelectBestSource(sources *amp.Tags, want SourceCriteria) *amp.Tag {
	if sources == nil {
		return nil
	}
	best := (*amp.Tag)(nil)
	for _, src := range sources.SubTags {
		if src == nil || src.IsNil() {
			continue
		}
		if want.Offline && !SourceIsBlob(src) {
			continue
		}
		if want.ContentType != "" && !strings.HasPrefix(src.ContentType(), strings.ToLower(want.ContentType)) {
			continue
		}
		if best == nil || sourceOutranks(src, best, want.MaxKbps) {
			best = src
		}
	}
	return best
}

// SourceIsBlob reports whether a source leaf points at a content-addressed
// blob (a UID with no URI).
func SourceIsBlob(src *amp.Tag) bool {
	return src != nil && src.URI == "" && !src.UID().IsNil()
}

// SourceScheme returns a source URI's lower-cased scheme ("https", "spotify",
// ...), or "" for a blob source or a scheme-less URI.
func SourceScheme(src *amp.Tag) string {
	if src == nil || src.URI == "" {
		return ""
	}
	split := strings.IndexByte(src.URI, ':')
	if split <= 0 {
		return ""
	}
	return strings.ToLower(src.URI[:split])
}

// sourceOutranks reports whether cand beats best under the bitrate cap:
// within-cap beats over-cap beats untagged; within-cap prefers higher,
// over-cap prefers lower; ties break on source kind (blob, then direct URL,
// then custom scheme).
func sourceOutranks(cand, best *amp.Tag, maxKbps int64) bool {
	candBucket, bestBucket := bitrateBucket(cand, maxKbps), bitrateBucket(best, maxKbps)
	if candBucket != bestBucket {
		return candBucket < bestBucket
	}
	candRate, bestRate := sourceKbps(cand), sourceKbps(best)
	if candRate != bestRate {
		if candBucket == 0 { // both within cap: higher is better
			return candRate > bestRate
		}
		if candBucket == 1 { // both over cap: lower overshoot is better
			return candRate < bestRate
		}
	}
	return sourceKind(cand) < sourceKind(best)
}

// bitrateBucket: 0 = tagged and within the cap, 1 = tagged over the cap,
// 2 = no bitrate tag.
func bitrateBucket(src *amp.Tag, maxKbps int64) int {
	rate := sourceKbps(src)
	switch {
	case rate == 0:
		return 2
	case maxKbps > 0 && rate > maxKbps:
		return 1
	default:
		return 0
	}
}

func sourceKbps(src *amp.Tag) int64 {
	if src.Units != amp.Units_BitrateKbps || src.I <= 0 {
		return 0
	}
	return src.I
}

// sourceKind: 0 = blob, 1 = direct URL (https/http/file), 2 = custom scheme.
func sourceKind(src *amp.Tag) int {
	if SourceIsBlob(src) {
		return 0
	}
	switch SourceScheme(src) {
	case "https", "http", "file":
		return 1
	default:
		return 2
	}
}
