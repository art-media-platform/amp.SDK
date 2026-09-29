package amp

// Marshal and read cost per frame shape: small ≈ the embedded rail's common
// frame (one op, 89-byte value), medium crosses 4 KiB, large is one 64 KiB
// value.  The libampd rail baselines (QD) are recorded from these.

import (
	"bytes"
	"testing"
)

type frameShape struct {
	name   string
	nOps   int
	valLen int
}

var frameShapes = []frameShape{
	{"small", 1, 89},
	{"medium4K", 16, 240},
	{"large64K", 1, 64 << 10},
}

// SendTx's marshal: fresh (nil dst → the first allocation) vs a reused buffer
// (a pool hit).
func BenchmarkMarshalToBuffer(b *testing.B) {
	for _, shape := range frameShapes {
		tx := txShape(shape.nOps, shape.valLen)
		b.Run(shape.name+"/fresh", func(b *testing.B) {
			b.ReportAllocs()
			var buf []byte
			for range b.N {
				buf = nil
				tx.MarshalToBuffer(&buf)
			}
			b.ReportMetric(float64(len(buf)), "frameB")
		})
		b.Run(shape.name+"/reuse", func(b *testing.B) {
			b.ReportAllocs()
			var buf []byte
			tx.MarshalToBuffer(&buf)
			for range b.N {
				tx.MarshalToBuffer(&buf)
			}
		})
	}
}

// A frame reader's parse (the TCP transport's shape): a fresh reader per frame.
func BenchmarkReadTxMsg(b *testing.B) {
	for _, shape := range frameShapes {
		var wire []byte
		txShape(shape.nOps, shape.valLen).MarshalToBuffer(&wire)
		b.Run(shape.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(wire)))
			for range b.N {
				if _, err := ReadTxMsg(bytes.NewReader(wire)); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
