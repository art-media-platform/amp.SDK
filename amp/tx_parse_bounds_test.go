package amp

// A frame parser's bounds on crafted lengths: a uvarint length that
// overflows int, or a skip past the span, is refused as malformed, never
// sliced.  These parsers run on the client's thread (the embedded rail) and
// on the TCP login path, where a panic ends a session or a process.

import (
	"bytes"
	"encoding/binary"
	"runtime"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/status"
)

// hugeUvarint is 2^63 as a uvarint: p + int(2^63) wraps negative.
var hugeUvarint = []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x01}

// frameWithHeadBytes returns a frame whose head is the preamble + body, with
// no data store: the preamble lengths agree with the frame.
func frameWithHeadBytes(body []byte) []byte {
	frame := make([]byte, int(TxPreambleSize)+len(body))
	copy(frame, TxPreambleSignature)
	binary.BigEndian.PutUint32(frame[4:8], uint32(len(frame)))
	binary.BigEndian.PutUint32(frame[8:12], 0)
	copy(frame[TxPreambleSize:], body)
	return frame
}

func TestOpenTx_HugeEnvelopeLengthIsMalformed(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("OpenTx panicked on a crafted envelope length: %v", recovered)
		}
	}()
	frame := frameWithHeadBytes(hugeUvarint)
	_, err := OpenTx(frame, nil, nil, safe.CryptoKitID{})
	if status.GetCode(err) != status.Code_MalformedTx {
		t.Fatalf("OpenTx = %v, want MalformedTx", err)
	}
	if _, err = ReadTxMsg(bytes.NewReader(frame)); status.GetCode(err) != status.Code_MalformedTx {
		t.Fatalf("ReadTxMsg = %v, want MalformedTx", err)
	}
}

// An op whose skip uvarint overflows int is refused before the walk slices past the span.
func TestOpenTx_HugeOpSkipIsMalformed(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("OpenTx panicked on a crafted op skip: %v", recovered)
		}
	}()
	// A valid frame first, then its one op's skip rewritten to 2^63.
	var wire []byte
	txShape(1, 8).MarshalToBuffer(&wire)
	envLen, n := binary.Uvarint(wire[TxPreambleSize:])
	p := int(TxPreambleSize) + n + int(envLen)
	hdrLen, n := binary.Uvarint(wire[p:])
	p += n + int(hdrLen)
	opsLen := int(binary.BigEndian.Uint32(wire[p:]))
	opStart := p + 4
	// op layout: flags, logical, dataOfs, dataLen, skip, ...
	q := opStart + 1
	for range 3 {
		_, n = binary.Uvarint(wire[q:])
		q += n
	}
	crafted := append([]byte{}, wire[:q]...)
	crafted = append(crafted, hugeUvarint...)
	crafted = append(crafted, wire[q+1:]...) // the original skip was one byte (0)
	binary.BigEndian.PutUint32(crafted[p:], uint32(opsLen+len(hugeUvarint)-1))
	headLen := int(binary.BigEndian.Uint32(crafted[4:8])) + len(hugeUvarint) - 1
	binary.BigEndian.PutUint32(crafted[4:8], uint32(headLen))

	_, err := OpenTx(crafted, nil, nil, safe.CryptoKitID{})
	if status.GetCode(err) != status.Code_MalformedTx {
		t.Fatalf("OpenTx = %v, want MalformedTx", err)
	}
}

// A refused frame allocates no TxMsg: the bounds run before anything is built.
func TestReadTxMsg_RefusesBeforeAllocatingTx(t *testing.T) {
	frame := frameClaiming(64, 5, 0)
	mallocsBefore := mallocs()
	if _, err := ReadTxMsg(bytes.NewReader(frame)); err == nil {
		t.Fatal("accepted")
	}
	if grew := mallocs() - mallocsBefore; grew > 2 {
		t.Fatalf("%d allocations for a refused frame; want at most the reader's own two", grew)
	}
}

func mallocs() uint64 {
	stats := &runtime.MemStats{}
	runtime.ReadMemStats(stats)
	return stats.Mallocs
}
