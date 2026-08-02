package amp_test

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

func testSealAddr(nodeID, attrID, itemID uint64) tag.Address {
	return tag.Address{
		ElementID: tag.ElementID{
			NodeID: tag.UID{nodeID, 0x11},
			AttrID: tag.UID{attrID, 0x22},
			ItemID: tag.UID{itemID, 0x33},
		},
	}
}

func TestSealedValueBoxRoundTrip(t *testing.T) {
	containerID := tag.UID{0x0700, 0x99}
	epochKey := testSymKey(t, tag.UID{0x0100, 0x42})
	addr := testSealAddr(1, 2, 3)
	plain := []byte("SG.fixture-not-a-real-key")

	box, err := amp.SealValueBox(rand.Reader, epochKey, containerID, addr, "text/plain", plain)
	if err != nil {
		t.Fatal(err)
	}
	if box.ContentType != "text/plain" {
		t.Fatalf("content type not carried: %q", box.ContentType)
	}
	if boxContainer := (tag.UID{box.ContainerID_0, box.ContainerID_1}); boxContainer != containerID {
		t.Fatalf("container not carried: %v", boxContainer)
	}
	if boxEpoch := (tag.UID{box.EpochID_0, box.EpochID_1}); boxEpoch != epochKey.EpochID {
		t.Fatalf("epoch not carried: %v", boxEpoch)
	}
	// Layout pin: nonce(24) ‖ ciphertext+tag(16) — drift breaks every stored record.
	if len(box.Ciphertext) != safe.NonceSize+len(plain)+16 {
		t.Fatalf("ciphertext layout drifted: %d bytes for %d plaintext", len(box.Ciphertext), len(plain))
	}

	opened, err := amp.OpenValueBox(epochKey, addr, box)
	if err != nil {
		t.Fatal(err)
	}
	defer safe.Zero(opened)
	if !bytes.Equal(opened, plain) {
		t.Fatal("opened plaintext != sealed plaintext")
	}
}

func TestSealedValueBoxRejects(t *testing.T) {
	containerID := tag.UID{0x0700, 0x99}
	epochKey := testSymKey(t, tag.UID{0x0100, 0x42})
	addr := testSealAddr(1, 2, 3)
	plain := []byte("SG.fixture-not-a-real-key")

	box, err := amp.SealValueBox(rand.Reader, epochKey, containerID, addr, "text/plain", plain)
	if err != nil {
		t.Fatal(err)
	}

	// A transplanted box MUST refuse to open: the element address is AAD.
	for _, transplanted := range []tag.Address{
		testSealAddr(9, 2, 3), // different NodeID
		testSealAddr(1, 9, 3), // different AttrID
		testSealAddr(1, 2, 9), // different ItemID
	} {
		if _, err := amp.OpenValueBox(epochKey, transplanted, box); err == nil {
			t.Fatalf("transplanted address %v opened", transplanted.ElementID)
		}
	}

	// Wrong key bytes under the right epoch UID: AEAD must refuse.
	forged := testSymKey(t, epochKey.EpochID)
	if _, err := amp.OpenValueBox(forged, addr, box); err == nil {
		t.Fatal("wrong key bytes opened")
	}

	// Key naming a different epoch: identity check must refuse.
	otherEpoch := testSymKey(t, tag.UID{0x0200, 0x42})
	if _, err := amp.OpenValueBox(otherEpoch, addr, box); err == nil {
		t.Fatal("mismatched epoch opened")
	}

	// Role mismatch: identity check must refuse.
	wrongRole := epochKey
	wrongRole.Role = safe.KeyRole_WriteSeed
	if _, err := amp.OpenValueBox(wrongRole, addr, box); err == nil {
		t.Fatal("mismatched role opened")
	}

	// Seal-side validation.
	if _, err := amp.SealValueBox(rand.Reader, safe.SymKey{}, containerID, addr, "text/plain", plain); err == nil {
		t.Fatal("empty key sealed")
	}
	if _, err := amp.SealValueBox(rand.Reader, epochKey, tag.UID{}, addr, "text/plain", plain); err == nil {
		t.Fatal("nil container sealed")
	}
	if _, err := amp.SealValueBox(rand.Reader, epochKey, containerID, addr, "", plain); err == nil {
		t.Fatal("empty content type sealed")
	}
}
