package link

import (
	"crypto/rand"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

func TestSealOpenRoundTrip(t *testing.T) {
	epochKey := make([]byte, 32)
	rand.Read(epochKey)

	tok := LinkToken{
		Version:   TokenVersion,
		PlanetID:  tag.NewID(),
		NodeID: tag.NewID(),
		ItemID:    tag.NewID(),
	}

	encoded, err := SealToken(tok, epochKey)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("token length: %d chars", len(encoded))

	got, err := OpenToken(encoded, epochKey)
	if err != nil {
		t.Fatal(err)
	}

	if got.Version != tok.Version {
		t.Errorf("version: got %d, want %d", got.Version, tok.Version)
	}
	if got.PlanetID != tok.PlanetID {
		t.Errorf("planetID mismatch")
	}
	if got.NodeID != tok.NodeID {
		t.Errorf("channelID mismatch")
	}
	if got.ItemID != tok.ItemID {
		t.Errorf("itemID mismatch")
	}
}

func TestWrongKeyFails(t *testing.T) {
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	rand.Read(key1)
	rand.Read(key2)

	tok := LinkToken{
		Version:   TokenVersion,
		PlanetID:  tag.NewID(),
		NodeID: tag.NewID(),
		ItemID:    tag.NewID(),
	}

	encoded, err := SealToken(tok, key1)
	if err != nil {
		t.Fatal(err)
	}

	_, err = OpenToken(encoded, key2)
	if err != ErrBadToken {
		t.Errorf("expected ErrBadToken, got: %v", err)
	}
}

func TestIsEncryptedToken(t *testing.T) {
	// Structured public path — not a token
	if IsEncryptedToken("ABC123/DEF456/GHI789") {
		t.Error("path with slashes should not be detected as token")
	}

	// Generate a real token to check detection
	epochKey := make([]byte, 32)
	rand.Read(epochKey)
	tok := LinkToken{Version: TokenVersion, PlanetID: tag.NewID(), NodeID: tag.NewID(), ItemID: tag.NewID()}
	encoded, err := SealToken(tok, epochKey)
	if err != nil {
		t.Fatal(err)
	}
	if !IsEncryptedToken(encoded) {
		t.Errorf("real token (len=%d) not detected", len(encoded))
	}

	// Too short
	if IsEncryptedToken("ABC123") {
		t.Error("short string should not be detected as token")
	}
}

func TestCorruptedTokenFails(t *testing.T) {
	epochKey := make([]byte, 32)
	rand.Read(epochKey)

	tok := LinkToken{Version: TokenVersion, PlanetID: tag.NewID(), NodeID: tag.NewID(), ItemID: tag.NewID()}
	encoded, err := SealToken(tok, epochKey)
	if err != nil {
		t.Fatal(err)
	}

	// Flip a character
	corrupted := []byte(encoded)
	corrupted[len(corrupted)/2] ^= 0xFF
	_, err = OpenToken(string(corrupted), epochKey)
	if err == nil {
		t.Error("expected error for corrupted token")
	}
}
