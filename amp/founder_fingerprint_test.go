package amp_test

import (
	"bytes"
	"encoding/hex"
	"maps"
	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// fingerprintFixture returns a fixed two-founder key set — literal bytes, never
// derived at runtime, so the golden below is a mint-once commitment
// (SD-channel-governance §8).
func fingerprintFixture() map[tag.UID]safe.PubKey {
	return map[tag.UID]safe.PubKey{
		{0x01, 0x02}: {
			CryptoKitID: safe.Crypto.Poly25519.ID,
			KeyType:     safe.KeyType_SigningKey,
			Bytes: []byte{
				0xA0, 0xA1, 0xA2, 0xA3, 0xA4, 0xA5, 0xA6, 0xA7,
				0xA8, 0xA9, 0xAA, 0xAB, 0xAC, 0xAD, 0xAE, 0xAF,
				0xB0, 0xB1, 0xB2, 0xB3, 0xB4, 0xB5, 0xB6, 0xB7,
				0xB8, 0xB9, 0xBA, 0xBB, 0xBC, 0xBD, 0xBE, 0xBF,
			},
		},
		{0x03, 0x04}: {
			CryptoKitID: safe.Crypto.P256.ID,
			KeyType:     safe.KeyType_SigningKey,
			Bytes: []byte{
				0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17,
				0x18, 0x19, 0x1A, 0x1B, 0x1C, 0x1D, 0x1E, 0x1F,
				0x20, 0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27,
				0x28, 0x29, 0x2A, 0x2B, 0x2C, 0x2D, 0x2E, 0x2F,
			},
		},
	}
}

// TestFounderFingerprint_Golden pins the fingerprint of the fixture founder
// set.  The digest is a permanent wire commitment carried by
// NameServiceRecord.FounderFingerprint / PlanetInvite.FounderFingerprint — a
// drift here breaks every pin ever issued, so a failure is either a bug or a
// deliberate v2 domain (never a re-bless of v1).
func TestFounderFingerprint_Golden(t *testing.T) {
	fp, err := amp.FounderFingerprint(fingerprintFixture(), 1)
	if err != nil {
		t.Fatal(err)
	}

	const goldenHex = "0beb6870e816c6be66715794fd2dc9dddc8f7417b7be2d25dffda833183f311d"

	if gotHex := hex.EncodeToString(fp); gotHex != goldenHex {
		t.Errorf("\nfounder-fingerprint drift\n got:  %s\nwant: %s", gotHex, goldenHex)
	}
}

// TestFounderFingerprint_OrderIndependence proves the commitment does not
// depend on founder map order: dropping and re-adding entries (perturbing Go's
// map iteration) yields the identical digest.
func TestFounderFingerprint_OrderIndependence(t *testing.T) {
	want, err := amp.FounderFingerprint(fingerprintFixture(), 1)
	if err != nil {
		t.Fatal(err)
	}
	for range 8 {
		rebuilt := make(map[tag.UID]safe.PubKey)
		maps.Copy(rebuilt, fingerprintFixture())
		got, err := amp.FounderFingerprint(rebuilt, 1)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("fingerprint depends on founder order: %x != %x", got, want)
		}
	}
}

// TestFounderFingerprint_Distinctness — changing the quorum, a key byte, or a
// key's kit each changes the commitment.
func TestFounderFingerprint_Distinctness(t *testing.T) {
	base, err := amp.FounderFingerprint(fingerprintFixture(), 1)
	if err != nil {
		t.Fatal(err)
	}

	quorum2, err := amp.FounderFingerprint(fingerprintFixture(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(base, quorum2) {
		t.Fatal("quorum change must change the fingerprint")
	}

	tamperedKey := fingerprintFixture()
	pub := tamperedKey[tag.UID{0x01, 0x02}]
	pub.Bytes = append([]byte(nil), pub.Bytes...)
	pub.Bytes[0] ^= 0xFF
	tamperedKey[tag.UID{0x01, 0x02}] = pub
	tampered, err := amp.FounderFingerprint(tamperedKey, 1)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(base, tampered) {
		t.Fatal("key-byte tamper must change the fingerprint")
	}

	rekitted := fingerprintFixture()
	pub = rekitted[tag.UID{0x01, 0x02}]
	pub.CryptoKitID = safe.Crypto.P256.ID
	rekitted[tag.UID{0x01, 0x02}] = pub
	kitChanged, err := amp.FounderFingerprint(rekitted, 1)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(base, kitChanged) {
		t.Fatal("kit change must change the fingerprint")
	}
}

// TestFounderFingerprint_Refusals — an empty founder set and a founder with no
// key bytes both error (a pin over nothing is not a commitment).
func TestFounderFingerprint_Refusals(t *testing.T) {
	if _, err := amp.FounderFingerprint(nil, 1); err == nil {
		t.Fatal("empty founder set must error")
	}
	keyless := fingerprintFixture()
	pub := keyless[tag.UID{0x01, 0x02}]
	pub.Bytes = nil
	keyless[tag.UID{0x01, 0x02}] = pub
	if _, err := amp.FounderFingerprint(keyless, 1); err == nil {
		t.Fatal("founder with no key bytes must error")
	}
}
