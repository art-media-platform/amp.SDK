package p256_test

// Golden byte-equality fixtures for the P-256 kit's encoded forms — pinned
// BEFORE any implementation motion (durable key identity is BYTES).  The
// pinned values were produced by the kit itself; every entry must keep
// verifying/opening byte-identically across any internal migration, and the
// SEC1-uncompressed public key derived from the pinned scalar must never move.

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	_ "github.com/art-media-platform/amp.SDK/stdlib/safe/p256" // register P-256 CryptoKit
)

// goldenScalar is a fixed in-range P-256 scalar (0x01..0x20).
func goldenScalar() []byte {
	prv := make([]byte, 32)
	for i := range prv {
		prv[i] = byte(i + 1)
	}
	return prv
}

const (
	// goldenPubHex is goldenScalar's SEC1-uncompressed public key
	// (0x04 || X || Y) — the registry/wire encoding.  Bytes are the identity:
	// this constant never moves.
	goldenPubHex = "04515c3d6eb9e396b904d3feca7f54fdcd0cc1e997bf375dca515ad0a6c3b4035f4536be3a50f318fbf9a5475902a221502bef0d57e08c53b2cc0a56f17d9f9354"

	// goldenSigHex is one ECDSA-P256 signature (raw r||s) over goldenMsg by
	// goldenScalar, produced once by the kit; Verify must accept it forever.
	goldenSigHex = "da740819037f7387c397b21780e4499464e0842aa935455154aa71346dc06212833fc56f54918f5f7384049cc35199b0cab26fd214fa1943e5b17988bd02b321"

	// goldenSealedHex is one sealed box (eph_pub || nonce || ct+tag) for
	// goldenScalar's public key, produced once by the kit; Open must recover
	// goldenPlaintext forever.
	goldenSealedHex = "04548495161e968040cfe89daeb12d71b70b6101fa71c876f4de3b10b18fadc547d6b5535c25ae45d7b20f2381d75b6a254a364fa4e47318b2d8a43c6ba80753dce446e059752fe38283fef0cdb0b71af69b30f583d6919c5d42bfa183640a1663ca81945bf99236fd0757b17f81ffad490ee6004f6780562c65be41105591c939df494585acca6cf1"
)

var (
	goldenMsg       = []byte("p256 golden fixture message (GRP-CONTRACTS 2026-08-16)")
	goldenPlaintext = []byte("p256 golden sealed-box plaintext")
)

func fromHex(t *testing.T, s string) []byte {
	t.Helper()
	raw, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("bad fixture hex: %v", err)
	}
	return raw
}

func goldenKit(t *testing.T) *safe.Kit {
	t.Helper()
	kit, err := safe.CryptoKit(safe.Crypto.P256.ID)
	if err != nil {
		t.Fatalf("CryptoKit(P256): %v", err)
	}
	return kit
}

// TestP256_Golden_PinnedSignatureVerifies: a signature minted under the
// pre-migration implementation stays valid — verify-side byte compatibility.
func TestP256_Golden_PinnedSignatureVerifies(t *testing.T) {
	kit := goldenKit(t)
	pub := fromHex(t, goldenPubHex)
	sig := fromHex(t, goldenSigHex)
	if err := kit.Signing.Verify(sig, goldenMsg, pub); err != nil {
		t.Fatalf("pinned signature must verify under pinned pub: %v", err)
	}
	bad := append([]byte{}, sig...)
	bad[7] ^= 0xFF
	if err := kit.Signing.Verify(bad, goldenMsg, pub); err == nil {
		t.Fatal("tampered pinned signature must reject")
	}
}

// TestP256_Golden_SignBindsToPinnedPub: a FRESH signature by the pinned
// scalar verifies under the PINNED public key — the sign path's internal
// scalar→point derivation is byte-equivalent to the pinned encoding.
func TestP256_Golden_SignBindsToPinnedPub(t *testing.T) {
	kit := goldenKit(t)
	pub := fromHex(t, goldenPubHex)
	sig, err := kit.Signing.Sign(goldenMsg, goldenScalar())
	if err != nil {
		t.Fatalf("Sign(goldenScalar): %v", err)
	}
	if err := kit.Signing.Verify(sig, goldenMsg, pub); err != nil {
		t.Fatalf("fresh signature by the pinned scalar must verify under the pinned pub: %v", err)
	}
}

// TestP256_Golden_PinnedSealedBoxOpens: a sealed box minted under the
// pre-migration implementation stays openable — open-side byte compatibility.
func TestP256_Golden_PinnedSealedBoxOpens(t *testing.T) {
	kit := goldenKit(t)
	opened, err := kit.Encrypt.Open(fromHex(t, goldenSealedHex), goldenScalar())
	if err != nil {
		t.Fatalf("pinned sealed box must open: %v", err)
	}
	if !bytes.Equal(opened, goldenPlaintext) {
		t.Fatalf("pinned sealed box opened to %q, want %q", opened, goldenPlaintext)
	}
}

// TestP256_Golden_SealToPinnedPubOpens: a FRESH seal to the pinned public key
// opens with the pinned scalar — the seal path's pub-key parse is
// byte-equivalent to the pinned encoding.
func TestP256_Golden_SealToPinnedPubOpens(t *testing.T) {
	kit := goldenKit(t)
	sealed, err := kit.Encrypt.Seal(rand.Reader, goldenPlaintext, fromHex(t, goldenPubHex))
	if err != nil {
		t.Fatalf("Seal(pinned pub): %v", err)
	}
	opened, err := kit.Encrypt.Open(sealed, goldenScalar())
	if err != nil {
		t.Fatalf("Open(fresh seal): %v", err)
	}
	if !bytes.Equal(opened, goldenPlaintext) {
		t.Fatalf("fresh seal opened to %q, want %q", opened, goldenPlaintext)
	}
}

// TestP256_Golden_PubKeyRejections pins the malformed-key rejection envelope:
// what rejects today must keep rejecting after any migration.
func TestP256_Golden_PubKeyRejections(t *testing.T) {
	kit := goldenKit(t)
	sig := fromHex(t, goldenSigHex)
	pub := fromHex(t, goldenPubHex)

	offCurve := append([]byte{}, pub...)
	offCurve[64] ^= 0x01 // valid shape, point not on the curve

	compressed := append([]byte{0x02}, pub[1:33]...) // SEC1 compressed — not the kit encoding

	cases := []struct {
		name string
		pub  []byte
	}{
		{"wrong length 64", pub[:64]},
		{"wrong length 66", append(append([]byte{}, pub...), 0x00)},
		{"compressed form", compressed},
		{"off-curve point", offCurve},
		{"all zero", make([]byte, 65)},
	}
	for _, tc := range cases {
		if err := kit.Signing.Verify(sig, goldenMsg, tc.pub); err == nil {
			t.Errorf("Verify(%s) must reject", tc.name)
		}
	}
}
