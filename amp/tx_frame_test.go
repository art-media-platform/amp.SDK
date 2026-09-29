package amp

// The wire-frame contract from a frame reader's vantage (a TCP peer, the
// embedded client): a preamble's lengths are bounded before anything is
// sized from them, allocations are exact, and the marshaller writes every
// preamble byte so nothing relies on a zeroed buffer.

import (
	"bytes"
	"encoding/binary"
	"runtime"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// frameClaiming returns a frameLen-byte frame whose preamble claims headLen and dataLen.
func frameClaiming(frameLen int, headLen uint32, dataLen uint32) []byte {
	frame := make([]byte, frameLen)
	copy(frame, TxPreambleSignature)
	binary.BigEndian.PutUint32(frame[4:8], headLen)
	binary.BigEndian.PutUint32(frame[8:12], dataLen)
	return frame
}

// txShape builds a tx of nOps ops, each carrying a valLen-byte value.
func txShape(nOps int, valLen int) *TxMsg {
	tx := TxNew()
	tx.SetTxID(tag.UID{0x1234567890abcdef, 0x0fedcba098765432})
	tx.SetPlanetID(tag.UID{0x1111222233334444, 0x5555666677778888})
	tx.SetContextID(tag.UID{0x3333, 0x4444})
	tx.SetFromID(tag.UID{0x5555, 0x6666})
	val := make([]byte, valLen)
	for i := range val {
		val[i] = byte(i)
	}
	for i := range nOps {
		op := TxOp{}
		op.Addr.NodeID = tag.UID{uint64(i + 1), 7}
		op.Addr.AttrID = tag.UID{9, 9}
		op.Addr.ItemID = tag.UID{uint64(i), uint64(i)}
		op.Flags = TxOpFlags_Upsert
		tx.MarshalOpAndData(&op, val)
	}
	return tx
}

func TestReadTxMsg_HeadShorterThanPreambleIsAnError(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("ReadTxMsg panicked on a malformed preamble: %v", recovered)
		}
	}()
	frame := frameClaiming(64, 5, 0)
	if _, err := ReadTxMsg(bytes.NewReader(frame)); err == nil {
		t.Fatal("ReadTxMsg accepted a head shorter than the preamble")
	}
}

// A preamble claiming more than the frame ceiling is refused before its
// lengths size an allocation.
func TestReadTxMsg_OverCeilingIsRefusedBeforeAllocating(t *testing.T) {
	cases := []struct {
		name  string
		frame []byte
	}{
		{"head over the ceiling", frameClaiming(64, uint32(TxMaxFrameSize)+1, 0)},
		{"data over the ceiling", frameClaiming(64, 32, uint32(TxMaxFrameSize)+1)},
		{"head + data over the ceiling", frameClaiming(64, uint32(TxMaxFrameSize)-8, 64)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := &runtime.MemStats{}
			runtime.ReadMemStats(before)
			_, err := ReadTxMsg(bytes.NewReader(tc.frame))
			after := &runtime.MemStats{}
			runtime.ReadMemStats(after)
			if err == nil {
				t.Fatal("ReadTxMsg accepted a frame over the ceiling")
			}
			if grew := after.TotalAlloc - before.TotalAlloc; grew > 1<<20 {
				t.Fatalf("ReadTxMsg allocated %d MiB for a %d-byte frame before refusing it", grew>>20, len(tc.frame))
			}
		})
	}
}

// A small frame is read into a buffer sized from its lengths, not a 2 KiB floor.
func TestReadTxMsg_ExactSizing(t *testing.T) {
	var wire []byte
	txShape(1, 89).MarshalToBuffer(&wire)
	preamble := TxPreamble(wire[:TxPreambleSize])
	need := max(preamble.TxHeadLen()-int(TxPreambleSize), preamble.TxDataLen())

	tx, err := ReadTxMsg(bytes.NewReader(wire))
	if err != nil {
		t.Fatal(err)
	}
	if cap(tx.DataStore) > need {
		t.Fatalf("DataStore cap = %d for a frame needing %d (head %d, data %d)", cap(tx.DataStore), need, preamble.TxHeadLen(), preamble.TxDataLen())
	}
}

// Marshaling into a reused buffer writes all 16 preamble bytes: 12-15 are
// zero after the marshal whatever the buffer held before.
func TestMarshalHeadAndOps_WritesEveryPreambleByte(t *testing.T) {
	buf := make([]byte, 0, 4096)
	for i := range buf[:cap(buf)] {
		buf[:cap(buf)][i] = 0xFF
	}
	txShape(1, 89).MarshalToBuffer(&buf)
	if !bytes.Equal(buf[12:16], []byte{0, 0, 0, 0}) {
		t.Fatalf("preamble bytes 12-15 = % x after marshaling into a dirty buffer; want 00 00 00 00", buf[12:16])
	}
}

// The first allocation is sized from the tx, not a 2 KiB floor.
func TestMarshalHeadAndOps_FirstAllocationFromCeiling(t *testing.T) {
	tx := txShape(1, 89)
	var buf []byte
	tx.MarshalToBuffer(&buf)
	t.Logf("frame %d bytes, ceiling %d, cap %d", len(buf), tx.CeilingSize(), cap(buf))
	if cap(buf) >= 2048 {
		t.Fatalf("cap = %d for a %d-byte frame (ceiling %d)", cap(buf), len(buf), tx.CeilingSize())
	}
}
