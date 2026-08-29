package amp_test

import (
	"bytes"
	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
)

// Hand-computed vectors pin the codec to values verifiable by hand — a
// symmetric encode/decode bug cannot survive them.
func TestBase58BTC_KnownVectors(t *testing.T) {
	cases := []struct {
		encoded string
		raw     []byte
	}{
		{"1", []byte{0x00}},         // leading '1' → zero byte
		{"2", []byte{0x01}},         // index 1
		{"z", []byte{0x39}},         // index 57
		{"21", []byte{0x3A}},        // 1*58 + 0 = 58
		{"211", []byte{0x0D, 0x24}}, // 1*58^2 = 3364 = 0x0D24
	}
	for _, c := range cases {
		got, err := amp.Base58BTCDecode(c.encoded)
		if err != nil {
			t.Fatalf("Base58BTCDecode(%q): %v", c.encoded, err)
		}
		if !bytes.Equal(got, c.raw) {
			t.Errorf("Base58BTCDecode(%q) = % x, want % x", c.encoded, got, c.raw)
		}
		if round := amp.Base58BTCEncode(c.raw); round != c.encoded {
			t.Errorf("Base58BTCEncode(% x) = %q, want %q", c.raw, round, c.encoded)
		}
	}

	// Characters outside the alphabet (0, O, I, l) are rejected.
	if _, err := amp.Base58BTCDecode("0OIl"); err == nil {
		t.Error("Base58BTCDecode accepted characters outside the alphabet")
	}
}
