package std

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/data"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// media_test.go proves the range-lazy contract of the media-stream reader without a
// host: a synthetic segment index over a counting BlobOpener.  The load-bearing
// assertion is that serving the tail of a long stream opens ONLY the segments the
// range touches — a four-hour mix is never read end-to-end into heap.

const (
	testSegLen   = 64 << 10
	testSegCount = 6
)

// readSeekNopCloser adapts a *bytes.Reader to data.AssetReader.
type readSeekNopCloser struct{ *bytes.Reader }

func (readSeekNopCloser) Close() error { return nil }

// countingOpener serves per-blob bytes and counts how many times each blob is opened.
type countingOpener struct {
	bytesByUID map[tag.UID][]byte
	opens      map[tag.UID]int
}

func (o *countingOpener) OpenBlob(_ tag.UID, ref *amp.BlobRef) (data.AssetReader, error) {
	uid := ref.AssetTag.UID()
	o.opens[uid]++
	payload, ok := o.bytesByUID[uid]
	if !ok {
		return nil, fmt.Errorf("countingOpener: no blob %s", uid.Base32())
	}
	return readSeekNopCloser{bytes.NewReader(payload)}, nil
}

// buildStream lays out testSegCount segments (each filled with a distinct byte) on a
// contiguous byte timeline and returns the index, total size, the expected
// concatenation, the per-segment blob UIDs, and a factory for fresh-count openers.
func buildStream(t *testing.T) (index []segEntry, size int64, concat []byte, uids []tag.UID, freshOpener func() *countingOpener) {
	t.Helper()
	bytesByUID := map[tag.UID][]byte{}
	index = make([]segEntry, testSegCount)
	uids = make([]tag.UID, testSegCount)
	offset := int64(0)
	for i := range testSegCount {
		uid := tag.NowID()
		payload := bytes.Repeat([]byte{byte('A' + i)}, testSegLen)
		bytesByUID[uid] = payload
		assetTag := &amp.Tag{I: testSegLen, Units: amp.Units_Bytes, ContentType: "audio/mpeg"}
		assetTag.SetID(uid)
		index[i] = segEntry{offset: offset, length: testSegLen, blob: &amp.BlobRef{AssetTag: assetTag}}
		uids[i] = uid
		concat = append(concat, payload...)
		offset += testSegLen
	}
	size = offset
	freshOpener = func() *countingOpener {
		return &countingOpener{bytesByUID: bytesByUID, opens: map[tag.UID]int{}}
	}
	return
}

func TestMediaStreamReader_FullReadConcatenatesEachSegmentOnce(t *testing.T) {
	index, size, concat, uids, freshOpener := buildStream(t)
	opener := freshOpener()
	reader := newMediaStreamReader(opener, tag.UID{}, index, size, 3*testSegLen)

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, concat) {
		t.Fatalf("full read = %d bytes, want %d (concatenation)", len(got), len(concat))
	}
	for i, uid := range uids {
		if opener.opens[uid] != 1 {
			t.Errorf("segment %d opened %d times, want exactly 1", i, opener.opens[uid])
		}
	}
	// Resident bytes stayed within the 3-segment budget despite reading all six.
	if reader.openSz > reader.residency {
		t.Errorf("resident bytes %d exceeded budget %d", reader.openSz, reader.residency)
	}
	reader.Close()
}

func TestMediaStreamReader_TailRangeOpensOnlyCoveringSegment(t *testing.T) {
	index, size, concat, uids, freshOpener := buildStream(t)
	opener := freshOpener()
	reader := newMediaStreamReader(opener, tag.UID{}, index, size, DefaultMediaResidentBytes)

	if _, err := reader.Seek(size-testSegLen, io.SeekStart); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	tail, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll(tail): %v", err)
	}
	if !bytes.Equal(tail, concat[size-testSegLen:]) {
		t.Fatalf("tail = %d bytes, want the last %d", len(tail), testSegLen)
	}
	// The heap proof: serving the tail touched ONLY the last segment.
	for i := range testSegCount - 1 {
		if opener.opens[uids[i]] != 0 {
			t.Errorf("tail range opened earlier segment %d (%d times) — not range-lazy", i, opener.opens[uids[i]])
		}
	}
	if opener.opens[uids[testSegCount-1]] != 1 {
		t.Errorf("tail segment opened %d times, want 1", opener.opens[uids[testSegCount-1]])
	}
	reader.Close()
}

func TestMediaStreamReader_SeekIsLazy(t *testing.T) {
	index, size, _, _, freshOpener := buildStream(t)
	opener := freshOpener()
	reader := newMediaStreamReader(opener, tag.UID{}, index, size, DefaultMediaResidentBytes)

	end, err := reader.Seek(0, io.SeekEnd)
	if err != nil {
		t.Fatalf("Seek(End): %v", err)
	}
	if end != size {
		t.Fatalf("Seek(End) = %d, want size %d", end, size)
	}
	totalOpens := 0
	for _, n := range opener.opens {
		totalOpens += n
	}
	if totalOpens != 0 {
		t.Errorf("Seek opened %d segments, want 0 (must be lazy)", totalOpens)
	}
	reader.Close()
}

func TestMediaStreamReader_CrossBoundaryRange(t *testing.T) {
	index, size, concat, uids, freshOpener := buildStream(t)
	opener := freshOpener()
	reader := newMediaStreamReader(opener, tag.UID{}, index, size, DefaultMediaResidentBytes)

	// A window straddling the boundary between segment 2 and segment 3.
	start := int64(2*testSegLen) + testSegLen/2
	span := int64(testSegLen) // half of seg 2 + half of seg 3
	if _, err := reader.Seek(start, io.SeekStart); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	got := make([]byte, span)
	if _, err := io.ReadFull(reader, got); err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
	if !bytes.Equal(got, concat[start:start+span]) {
		t.Fatalf("cross-boundary read mismatch")
	}
	// Exactly segments 2 and 3 were opened; no others.
	for i, uid := range uids {
		want := 0
		if i == 2 || i == 3 {
			want = 1
		}
		if opener.opens[uid] != want {
			t.Errorf("segment %d opened %d times, want %d", i, opener.opens[uid], want)
		}
	}
	reader.Close()
}
