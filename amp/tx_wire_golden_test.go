package amp

// Golden wire vectors for the TxOp codec, the value-header TxID carrier, and
// EditID reconstruction (SD-edit-resolution §6.1).  The hex literals are
// duplicated VERBATIM in C# (amp.3D.unity .../generic/Editor/TxWire_Tests.cs)
// — Go and C# must stay bit-identical; change one side only with the other.

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// Fixed identities shared by every vector (and the C# mirror).
var (
	goldenTxID   = tag.UID{0x1122334455667788, 0x99aabbccddeeff00}
	goldenFromID = tag.UID{0x0102030405060708, 0x090a0b0c0d0e0f10}
	authorTxID_A = tag.UID{0xa1a2a3a4a5a6a7a8, 0xb1b2b3b4b5b6b7b8}
)

// goldenOps builds the fixed 3-op shape: op2 repeats op1's NodeID/AttrID
// (delta mask = ItemID bits only), op3 changes AttrID + ItemID.
func goldenOps() []TxOp {
	op1 := TxOp{
		Flags:   TxOpFlags_Upsert,
		Logical: 0,
		DataOfs: 0,
		DataLen: 17,
	}
	op1.Addr.NodeID = tag.UID{0x1000000000000001, 0x2000000000000002}
	op1.Addr.AttrID = tag.UID{0x3000000000000003, 0x4000000000000004}
	op1.Addr.ItemID = tag.UID{0x5000000000000005, 0x6000000000000006}

	op2 := op1
	op2.Addr.ItemID = tag.UID{0x5000000000000007, 0x6000000000000008}
	op2.DataOfs = 17
	op2.DataLen = 33

	op3 := op2
	op3.Flags = TxOpFlags_Delete
	op3.Logical = 3
	op3.Addr.AttrID = tag.UID{0x3000000000000009, 0x400000000000000a}
	op3.Addr.ItemID = tag.UID{0x500000000000000b, 0x600000000000000c}
	op3.DataOfs = 50
	op3.DataLen = 0

	return []TxOp{op1, op2, op3}
}

// TestTxOpCodecGolden locks the ops-section byte layout: no per-op EditID,
// TxField slots ItemID_0=0 … NodeID_1=5, hasFields delta mask.
func TestTxOpCodecGolden(t *testing.T) {
	const goldenOpsHex = "0000007204000011003f5000000000000005600000000000000630000000000000034000000000000004100000000000000120000000000000020400112100035000000000000007600000000000000808033200000f500000000000000b600000000000000c3000000000000009400000000000000a"

	opsSection := appendOps(nil, goldenOps())
	gotHex := hex.EncodeToString(opsSection)
	if gotHex != goldenOpsHex {
		t.Errorf("\nops-section drift\n got:  %s\nwant: %s\n\nThe op codec is a frozen wire shape (v0.260.0); update the golden only for a deliberate pre-freeze amendment, and update the C# mirror in the same arc.", gotHex, goldenOpsHex)
	}

	// Round-trip: decode restores every field; EditID is zero here (decode-exit
	// reconstruction is exercised in TestEditIDReconstructionGolden).
	tx := TxNew()
	pos := 0
	if err := readOpsSection(tx, opsSection, &pos); err != nil {
		t.Fatalf("readOpsSection: %v", err)
	}
	want := goldenOps()
	if len(tx.Ops) != len(want) {
		t.Fatalf("decoded %d ops, want %d", len(tx.Ops), len(want))
	}
	for i, wantOp := range want {
		gotOp := tx.Ops[i]
		wantOp.Addr.EditID = tag.UID{}
		if gotOp != wantOp {
			t.Errorf("op %d mismatch:\n got:  %+v\nwant: %+v", i, gotOp, wantOp)
		}
	}
}

// TestEditIDReconstructionGolden locks the §6.1 identity rule end-to-end:
// an op whose value header carries ValueHeaderFlags_TxID restores that
// authoring TxID; an op without it restores the envelope TxID.
func TestEditIDReconstructionGolden(t *testing.T) {
	tx := TxNew()
	tx.SetTxID(goldenTxID)
	tx.SetFromID(goldenFromID)
	tx.SetContextID(tag.UID{0x1, 0x2})
	tx.Status = PinStatus_Synced

	baseAddr := tag.Address{}
	baseAddr.NodeID = tag.UID{0x1000000000000001, 0x2000000000000002}
	baseAddr.AttrID = tag.UID{0x3000000000000003, 0x4000000000000004}

	// Op 1: live shape — FromID-only header; identity = envelope TxID.
	liveValue := []byte{byte(ValueHeaderFlags_FromID)}
	liveValue = goldenFromID.AppendTo(liveValue)
	liveValue = append(liveValue, 0xde, 0xad)
	op1 := TxOp{Flags: TxOpFlags_Upsert, Addr: baseAddr}
	op1.Addr.ItemID = tag.UID{0x5000000000000005, 0x6000000000000006}
	tx.MarshalOpAndData(&op1, liveValue)

	// Op 2: served shape — FromID+TxID header (0x03); identity = header TxID.
	servedValue := []byte{byte(ValueHeaderFlags_FromID | ValueHeaderFlags_TxID)}
	servedValue = goldenFromID.AppendTo(servedValue)
	servedValue = authorTxID_A.AppendTo(servedValue)
	servedValue = append(servedValue, 0xbe, 0xef)
	op2 := TxOp{Flags: TxOpFlags_Upsert, Addr: baseAddr}
	op2.Addr.ItemID = tag.UID{0x5000000000000007, 0x6000000000000008}
	tx.MarshalOpAndData(&op2, servedValue)

	// Op 3: TxID-only header (0x02) — header UID is first; identity = header TxID.
	bareValue := []byte{byte(ValueHeaderFlags_TxID)}
	bareValue = authorTxID_A.AppendTo(bareValue)
	op3 := TxOp{Flags: TxOpFlags_Upsert, Addr: baseAddr}
	op3.Addr.ItemID = tag.UID{0x500000000000000b, 0x600000000000000c}
	tx.MarshalOpAndData(&op3, bareValue)

	var wire []byte
	tx.MarshalToBuffer(&wire)

	// The full-tx wire fixture: the C# mirror decodes exactly these bytes
	// (its sessReader path) and must reconstruct the same three EditIDs.
	const goldenReconstructionTxHex = "414d5031000000b00000004700000000120988776655443322111100ffeeddccbbaa992609010000000000000011020000000000000019080706050403020121100f0e0d0c0b0a0938090000006204000013003f50000000000000056000000000000006300000000000000340000000000000041000000000000001200000000000000204001323000350000000000000076000000000000008040036110003500000000000000b600000000000000c010102030405060708090a0b0c0d0e0f10dead030102030405060708090a0b0c0d0e0f10a1a2a3a4a5a6a7a8b1b2b3b4b5b6b7b8beef02a1a2a3a4a5a6a7a8b1b2b3b4b5b6b7b8"
	if gotHex := hex.EncodeToString(wire); gotHex != goldenReconstructionTxHex {
		t.Errorf("\nreconstruction-tx wire drift\n got:  %s\nwant: %s\n\nUpdate the C# mirror in the same arc.", gotHex, goldenReconstructionTxHex)
	}

	decoded, err := ReadTxMsg(&bufReader{buf: wire})
	if err != nil {
		t.Fatalf("ReadTxMsg: %v", err)
	}
	wantEditIDs := []tag.UID{goldenTxID, authorTxID_A, authorTxID_A}
	for i, want := range wantEditIDs {
		if got := decoded.Ops[i].Addr.EditID; got != want {
			t.Errorf("op %d EditID = %v, want %v", i, got, want)
		}
	}
}

// TestDeriveIDParityGolden pins the DeriveID/Midpoint derivation both
// languages share (the BUG-A class, SD-edit-resolution §4.1).
func TestDeriveIDParityGolden(t *testing.T) {
	if got := goldenTxID.DeriveID(tag.UID{}); got != goldenTxID {
		t.Errorf("DeriveID(zero) = %v, want the ID itself", got)
	}

	const goldenMidpointHex = "59626b747d868f98a5aeb7c0c9d2db5c"
	derived := goldenTxID.DeriveID(authorTxID_A)
	gotHex := hex.EncodeToString(derived.AppendTo(nil))
	if gotHex != goldenMidpointHex {
		t.Errorf("DeriveID(seeded) = %s, want %s", gotHex, goldenMidpointHex)
	}
	if !bytes.Equal(derived.AppendTo(nil), derived.AppendTo(nil)) {
		t.Error("AppendTo not deterministic")
	}
}
