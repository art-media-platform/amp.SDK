package amp

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/data"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

func TestTxSerialize(t *testing.T) {
	// Test serialization of a simple TxMsg

	tx := TxNew()
	tx.SetTxID(tag.UID{0x123456bcdef, 0xabdef127890})
	tx.SetFromID(tag.UID{0x11111, 0x22222})

	tx.Status = PinStatus_Syncing
	tx.SetContextID(tag.UID{0x1234567890abcdef, 0xabcdef1234567890})

	{
		op := TxOp{
			Flags: TxOpFlags_Upsert,
			Addr: tag.Address{
				ElementID: tag.ElementID{
					NodeID: tag.UID{99923456789, 987621},
					AttrID: tag.UID{111312232, 22232334444},
					ItemID: tag.UID{73833773, 76549},
				},
				EditID: tag.UID{4435435, 83849854543},
			},
		}

		tx.MarshalOp(&op, &Login{
			Member: &Tag{
				Text: "astar incoming",
				URI:  "drewz://soon.com/hawt#incoming",
			},
			HostAddress: "batwing ave",
		})
		tx.DataStore = append(tx.DataStore, []byte("bytes not used but stored -- not normal!")...)

		op.Addr.NodeID[0] -= 4321
		op.Addr.NodeID[1] += 37733773
		op.Addr.AttrID[0] -= 50454123
		op.Addr.ItemID[1] *= 745983
		op.Addr.EditID[0] += 123456789
		op.Addr.EditID[1] *= 0xbeef

		data := []byte("hello-world")
		for i := 0; i < 7; i++ {
			data = append(data, data...)
		}
		tx.MarshalOp(&op, &Login{
			Member: &Tag{
				URI:  "http://localhost:8080",
				Text: "what are we even doing here",
			},
			HostAddress: "http://localhost:8080",
		})

		for i := 0; i < 5500; i++ {
			op.Addr.ItemID[0] = uint64(i)
			if i%5 == 0 {
				op.Addr.EditID[1] += 37
			}
			tx.MarshalOp(&op, &LoginResponse{
				HashResponse: append(data, fmt.Sprintf("-%d", i)...),
			})
		}

		op.Addr.ItemID[0] = 111111
		op.Addr.EditID[1] = 55445544
		tx.MarshalOpAndData(&op, nil)
	}

	var txBuf []byte
	tx.MarshalToBuffer(&txBuf)

	r := bufReader{
		buf: txBuf,
	}
	t2, err := ReadTxMsg(&r)
	if err != nil {
		t.Errorf("ReadTxMsg failed: %v", err)
	}
	e1, _ := data.MarshalTo(nil, &tx.TxEnvelope)
	e2, _ := data.MarshalTo(nil, &t2.TxEnvelope)
	if !bytes.Equal(e1, e2) {
		t.Errorf("ReadTxMsg failed: TxEnvelope mismatch")
	}

	h1, _ := data.MarshalTo(nil, &tx.TxHeader)
	h2, _ := data.MarshalTo(nil, &t2.TxHeader)
	if !bytes.Equal(h1, h2) {
		t.Errorf("ReadTxMsg failed: TxHeader mismatch")
	}
	if len(tx.Ops) != len(t2.Ops) {
		t.Errorf("ReadTxMsg failed: TxHeader mismatch")
	}

	if len(tx.Ops) != len(t2.Ops) {
		t.Errorf("ReadTxMsg failed: TxEnvelope mismatch")
	}

	if !bytes.Equal(tx.DataStore, t2.DataStore) {
		t.Errorf("ReadTxMsg failed: DataStore mismatch")
	}
	for i, op1 := range tx.Ops {
		op2 := t2.Ops[i]

		if op1.Flags != op2.Flags || op1 != op2 || op1.DataOfs != op2.DataOfs || op1.DataLen != op2.DataLen {
			t.Errorf("ReadTxMsg failed: TxOp mismatch")
		}
	}
}

// TestTxWithinGracePeriod exercises the revocation-cliff math: after a member
// is suspended or revoked under a new epoch, TxMsgs they authored before that
// epoch's timestamp remain acceptable for MaxGracePeriod seconds and are rejected
// after.  This is the "90-day wind-down" window that lets a departing member
// publish a final handoff, while capping how long a compromised key can forge
// authority after rotation.
func TestTxWithinGracePeriod(t *testing.T) {
	mkUID := func(unix int64) tag.UID {
		return tag.UID{uint64(unix) << 16, 0}
	}

	epochTime := int64(1_700_000_000)
	epochID := mkUID(epochTime)

	cases := []struct {
		name      string
		epoch     *PlanetEpoch
		txTime    int64
		wantAllow bool
	}{
		{"future tx (tx >= epoch)", nil, epochTime + 1, true},
		{"concurrent tx (tx == epoch)", nil, epochTime, true},
		{"inside default grace (89 days)", nil, epochTime - 89*86400, true},
		{"at default grace boundary (90 days)", nil, epochTime - 90*86400, true},
		{"outside default grace (91 days)", nil, epochTime - 91*86400, false},
		{"custom short grace 7 days — inside", &PlanetEpoch{MaxGracePeriod: 7 * 86400}, epochTime - 6*86400, true},
		{"custom short grace 7 days — outside", &PlanetEpoch{MaxGracePeriod: 7 * 86400}, epochTime - 8*86400, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.epoch.TxWithinGracePeriod(mkUID(tc.txTime), epochID)
			if got != tc.wantAllow {
				t.Fatalf("TxWithinGracePeriod(tx=%d, epoch=%d, grace=%d) = %v, want %v",
					tc.txTime, epochTime, tc.epoch.GracePeriod(), got, tc.wantAllow)
			}
		})
	}
}

type bufReader struct {
	buf []byte
	pos int
}

func (r *bufReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.buf) {
		return 0, io.EOF
	}
	n = copy(p, r.buf[r.pos:])
	r.pos += n
	return n, nil
}
