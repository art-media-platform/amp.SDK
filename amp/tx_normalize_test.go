package amp

// Fixtures for the Normalize validation contract (backlog-ratified): hostile
// and malformed TxMsgs assert reject-vs-normalize per the contract stated at
// the function.

import (
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

func normalizeOp(node, attr, item, edit uint64, dataOfs, dataLen uint64) TxOp {
	op := TxOp{
		Flags:   TxOpFlags_Upsert,
		DataOfs: dataOfs,
		DataLen: dataLen,
	}
	op.Addr.NodeID = tag.UID{node, 1}
	op.Addr.AttrID = tag.UID{attr, 1}
	op.Addr.ItemID = tag.UID{item, 1}
	if edit != 0 {
		op.Addr.EditID = tag.UID{edit, 1}
	}
	return op
}

func TestNormalize_Contract(t *testing.T) {
	store := make([]byte, 64)

	cases := []struct {
		name string
		tx   *TxMsg
		ok   bool
	}{
		{"empty tx", &TxMsg{}, true},
		{"sorted distinct ops", &TxMsg{
			DataStore: store,
			Ops: []TxOp{
				normalizeOp(1, 1, 1, 9, 0, 32),
				normalizeOp(1, 1, 2, 9, 32, 32),
			},
		}, true},
		{"unsorted ops normalize", &TxMsg{
			DataStore: store,
			Ops: []TxOp{
				normalizeOp(1, 1, 2, 9, 32, 32),
				normalizeOp(1, 1, 1, 9, 0, 32),
			},
		}, true},
		{"nil EditID rejects", &TxMsg{
			DataStore: store,
			Ops:       []TxOp{normalizeOp(1, 1, 1, 0, 0, 32)},
		}, false},
		{"span past DataStore rejects", &TxMsg{
			DataStore: store,
			Ops:       []TxOp{normalizeOp(1, 1, 1, 9, 32, 33)},
		}, false},
		{"span uint64 wrap rejects", &TxMsg{
			DataStore: store,
			Ops:       []TxOp{normalizeOp(1, 1, 1, 9, ^uint64(0)-8, 16)},
		}, false},
		{"zero-length span at end ok", &TxMsg{
			DataStore: store,
			Ops:       []TxOp{normalizeOp(1, 1, 1, 9, 64, 0)},
		}, true},
		{"duplicate full address rejects", &TxMsg{
			DataStore: store,
			Ops: []TxOp{
				normalizeOp(1, 1, 1, 9, 0, 32),
				normalizeOp(1, 1, 1, 9, 32, 32),
			},
		}, false},
		{"same element, distinct EditIDs ok", &TxMsg{ // the sibling-serve shape
			DataStore: store,
			Ops: []TxOp{
				normalizeOp(1, 1, 1, 8, 0, 32),
				normalizeOp(1, 1, 1, 9, 32, 32),
			},
		}, true},
	}
	for _, tc := range cases {
		err := tc.tx.Normalize(true)
		if (err == nil) != tc.ok {
			t.Errorf("%s: err = %v, want ok=%v", tc.name, err, tc.ok)
		}
		if err == nil && !tc.tx.Normalized {
			t.Errorf("%s: Normalized flag not set on success", tc.name)
		}
	}

	// Post-normalize order: strictly ascending by full Address.
	sorted := &TxMsg{
		DataStore: store,
		Ops: []TxOp{
			normalizeOp(2, 1, 1, 9, 0, 8),
			normalizeOp(1, 1, 2, 9, 8, 8),
			normalizeOp(1, 1, 1, 9, 16, 8),
		},
	}
	if err := sorted.Normalize(true); err != nil {
		t.Fatalf("Normalize(sortable): %v", err)
	}
	for i := 1; i < len(sorted.Ops); i++ {
		if sorted.Ops[i-1].Addr.Compare(&sorted.Ops[i].Addr) >= 0 {
			t.Fatalf("ops not strictly ascending after Normalize: %d vs %d", i-1, i)
		}
	}
}

// TestNormalize_WireNeverPreNormalized: contract invariant 4 — a decode exit
// always resets Normalized, so a hostile wire image cannot assert it.
func TestNormalize_WireNeverPreNormalized(t *testing.T) {
	tx := TxNew()
	tx.SetTxID(tag.NowID())
	if err := tx.Upsert(tag.UID{1, 1}, tag.UID{2, 2}, tag.UID{3, 3}, nil); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := tx.Normalize(true); err != nil {
		t.Fatalf("Normalize: %v", err)
	}

	wire := []byte(nil)
	tx.MarshalToBuffer(&wire)
	decoded, err := OpenTx(wire, nil, nil, safe.CryptoKitID{})
	if err != nil {
		t.Fatalf("OpenTx: %v", err)
	}
	if decoded.Normalized {
		t.Fatal("a decoded TxMsg must never arrive pre-normalized")
	}
}
