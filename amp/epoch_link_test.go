package amp_test

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/amp/std"
	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// GOLDEN: amp.law.EpochLink minted 2026-07-12 (mint-once; identity is bytes).
// A forge-derivation change that moves this UID breaks every journaled
// EpochLink record — this guard fails the build the moment it drifts.
func TestLawEpochLinkGolden(t *testing.T) {
	golden := tag.UID{0xC320666D94AB0CFA, 0xD04D554E454713BF}
	if std.Attr.LawEpochLink.ID != golden {
		t.Fatalf("LawEpochLink UID drifted: %v != golden %v", std.Attr.LawEpochLink.ID, golden)
	}
}

func testSymKey(t *testing.T, epochID tag.UID) safe.SymKey {
	t.Helper()
	keyBytes := make([]byte, safe.DEKSize)
	if _, err := rand.Read(keyBytes); err != nil {
		t.Fatal(err)
	}
	return safe.SymKey{
		CryptoKitID: safe.Crypto.Poly25519.ID,
		EpochID:     epochID,
		Role:        safe.KeyRole_ContentKey,
		Bytes:       keyBytes,
	}
}

func TestEpochLinkBoxRoundTrip(t *testing.T) {
	// Explicit time-ordered UIDs: NowID is not strictly monotonic within one
	// clock tick (entropy bits), and seal enforces ToEpoch < FromEpoch.
	olderID := tag.UID{0x0100, 0x42}
	newerID := tag.UID{0x0200, 0x42}
	older := testSymKey(t, olderID)
	newer := testSymKey(t, newerID)

	box, err := amp.SealEpochLinkBox(rand.Reader, newer, older)
	if err != nil {
		t.Fatal(err)
	}
	link := &amp.EpochLink{
		FromEpoch: amp.TagFromUID(newerID),
		ToEpoch:   amp.TagFromUID(olderID),
		Box:       box,
	}

	opened, err := amp.OpenEpochLinkBox(newer, link)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Zero()
	if !bytes.Equal(opened.Bytes, older.Bytes) {
		t.Fatal("opened key != sealed key")
	}
	if opened.EpochID != olderID || opened.Role != safe.KeyRole_ContentKey {
		t.Fatalf("opened key identity wrong: epoch %v role %v", opened.EpochID, opened.Role)
	}
	if opened.CryptoKitID != older.CryptoKitID {
		t.Fatalf("opened key kit wrong: %v", opened.CryptoKitID)
	}
}

func TestEpochLinkBoxRejects(t *testing.T) {
	epoch1 := tag.UID{0x0100, 0x42}
	epoch2 := tag.UID{0x0200, 0x42}
	epoch3 := tag.UID{0x0300, 0x42}
	key1 := testSymKey(t, epoch1)
	key2 := testSymKey(t, epoch2)
	key3 := testSymKey(t, epoch3)

	// Seal-side: ToEpoch must strictly predate FromEpoch.
	if _, err := amp.SealEpochLinkBox(rand.Reader, key1, key2); err == nil {
		t.Fatal("sealed a forward link (ToEpoch newer than FromEpoch)")
	}
	if _, err := amp.SealEpochLinkBox(rand.Reader, key1, key1); err == nil {
		t.Fatal("sealed a self link")
	}

	box, err := amp.SealEpochLinkBox(rand.Reader, key2, key1)
	if err != nil {
		t.Fatal(err)
	}

	// Open with the wrong FromEpoch key: refused before any AEAD pass.
	link := &amp.EpochLink{
		FromEpoch: amp.TagFromUID(epoch2),
		ToEpoch:   amp.TagFromUID(epoch1),
		Box:       box,
	}
	if _, err := amp.OpenEpochLinkBox(key3, link); err == nil {
		t.Fatal("opened with a mismatched FromEpoch key")
	}

	// Transplant: the same box re-labeled under a different link must fail
	// (both epoch UIDs bind the AEAD as AAD).
	transplant := &amp.EpochLink{
		FromEpoch: amp.TagFromUID(epoch3),
		ToEpoch:   amp.TagFromUID(epoch1),
		Box:       box,
	}
	transplant.Box.EpochID_0 = epoch1[0]
	transplant.Box.EpochID_1 = epoch1[1]
	if _, err := amp.OpenEpochLinkBox(key3, transplant); err == nil {
		t.Fatal("opened a transplanted box under a different FromEpoch")
	}

	// Truncated box.
	short := &amp.EpochLink{
		FromEpoch: amp.TagFromUID(epoch2),
		ToEpoch:   amp.TagFromUID(epoch1),
		Box: &safe.EncryptedSymKey{
			EpochID_0:  epoch1[0],
			EpochID_1:  epoch1[1],
			Ciphertext: box.Ciphertext[:8],
		},
	}
	if _, err := amp.OpenEpochLinkBox(key2, short); err == nil {
		t.Fatal("opened a truncated box")
	}
}
