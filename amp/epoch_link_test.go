package amp_test

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/amp/std"
	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"google.golang.org/protobuf/proto"
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

	// Transplant: the same box re-labeled under a different link must fail on
	// the AAD alone.  The opening key keeps key2's bytes, so the subkey, nonce
	// and ciphertext match the seal; only the epoch UIDs in the AAD differ.
	relabeledFrom := key2
	relabeledFrom.EpochID = epoch3
	transplantFrom := &amp.EpochLink{
		FromEpoch: amp.TagFromUID(epoch3),
		ToEpoch:   amp.TagFromUID(epoch1),
		Box:       proto.Clone(box).(*safe.EncryptedSymKey),
	}
	_, err = amp.OpenEpochLinkBox(relabeledFrom, transplantFrom)
	if err == nil {
		t.Fatal("opened a transplanted box under a different FromEpoch")
	}
	epoch0 := tag.UID{0x0080, 0x42}
	transplantTo := &amp.EpochLink{
		FromEpoch: amp.TagFromUID(epoch2),
		ToEpoch:   amp.TagFromUID(epoch0),
		Box:       proto.Clone(box).(*safe.EncryptedSymKey),
	}
	transplantTo.Box.EpochID_0 = epoch0[0]
	transplantTo.Box.EpochID_1 = epoch0[1]
	if _, err := amp.OpenEpochLinkBox(key2, transplantTo); err == nil {
		t.Fatal("opened a transplanted box under a different ToEpoch")
	}
	// Control: the untouched link still opens, so the refusals above are the
	// AAD's.
	opened, err := amp.OpenEpochLinkBox(key2, link)
	if err != nil {
		t.Fatalf("the untransplanted link must open: %v", err)
	}
	opened.Zero()

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

// TestEpochLinkBox_Golden pins an EpochLink box for fixed keys and a fixed
// nonce: the HKDF purpose, the AAD (FromEpoch ‖ ToEpoch), and the box layout
// (nonce ‖ ciphertext ‖ tag).  A published box is durable wire output; a
// change here strands every journaled EpochLink.
//
// Oracle (no Go): subkey = HKDF-SHA256(0x11×32, no salt, "epoch-link") =
// b4ddc15e…8f71ba71 (Python hmac); box = nonce ‖ XChaCha20-Poly1305(subkey,
// nonce 0x33×24, aad u64BE(0x0200) u64BE(0x42) u64BE(0x0100) u64BE(0x42),
// 0x22×32), with HChaCha20 in Python (checked against the
// draft-irtf-cfrg-xchacha §2.2.1 vector) and ChaCha20-Poly1305 by OpenSSL 3
// (checked against RFC 8439 §2.8.2).
func TestEpochLinkBox_Golden(t *testing.T) {
	fromEpoch := tag.UID{0x0200, 0x42}
	toEpoch := tag.UID{0x0100, 0x42}
	fromKey := safe.SymKey{
		CryptoKitID: safe.Crypto.Poly25519.ID,
		EpochID:     fromEpoch,
		Role:        safe.KeyRole_ContentKey,
		Bytes:       bytes.Repeat([]byte{0x11}, safe.DEKSize),
	}
	toKey := safe.SymKey{
		CryptoKitID: safe.Crypto.Poly25519.ID,
		EpochID:     toEpoch,
		Role:        safe.KeyRole_ContentKey,
		Bytes:       bytes.Repeat([]byte{0x22}, safe.DEKSize),
	}
	nonce := bytes.NewReader(bytes.Repeat([]byte{0x33}, safe.NonceSize))

	const goldenBoxHex = "333333333333333333333333333333333333333333333333" +
		"fd250155df09a1614473570366d0ee19bcdb6204a2235c957d571503e4f16004" +
		"4a03585cec8e127b7147d735d35f17a2"

	box, err := amp.SealEpochLinkBox(nonce, fromKey, toKey)
	if err != nil {
		t.Fatal(err)
	}
	if gotHex := hex.EncodeToString(box.Ciphertext); gotHex != goldenBoxHex {
		t.Errorf("\nEpochLink box drift\n got:  %s\nwant: %s",
			gotHex, goldenBoxHex)
	}
	boxKit := safe.CryptoKitID{box.CryptoKitID_0, box.CryptoKitID_1}
	if boxKit != safe.Crypto.Poly25519.ID {
		t.Errorf("box CryptoKitID = %v, want Poly25519", boxKit)
	}
	boxEpoch := tag.UID{box.EpochID_0, box.EpochID_1}
	if boxEpoch != toEpoch {
		t.Errorf("box EpochID = %v, want ToEpoch %v", boxEpoch, toEpoch)
	}

	ciphertext, err := hex.DecodeString(goldenBoxHex)
	if err != nil {
		t.Fatal(err)
	}
	link := &amp.EpochLink{
		FromEpoch: amp.TagFromUID(fromEpoch),
		ToEpoch:   amp.TagFromUID(toEpoch),
		Box: &safe.EncryptedSymKey{
			CryptoKitID_0: safe.Crypto.Poly25519.ID[0],
			CryptoKitID_1: safe.Crypto.Poly25519.ID[1],
			EpochID_0:     toEpoch[0],
			EpochID_1:     toEpoch[1],
			Ciphertext:    ciphertext,
		},
	}
	opened, err := amp.OpenEpochLinkBox(fromKey, link)
	if err != nil {
		t.Fatalf("the golden box does not open: %v", err)
	}
	defer opened.Zero()
	if !bytes.Equal(opened.Bytes, toKey.Bytes) {
		t.Errorf("golden box opened to %x, want %x", opened.Bytes, toKey.Bytes)
	}
}
