package poly25519

import (
	"bytes"
	"crypto/ecdh"
	"encoding/hex"
	"testing"
)

// Golden derived-key bytes for fixed keypairs.  Pins the full ECDH→HKDF
// derivation (shared secret + canonical-ordered pubkey info) so any change to
// the derivation path is a test failure, not a silent interop break.
const x25519DeriveGolden = "d0513930bc7a81b6629c834eb727e8edd35ef705eb9e28d24b7e0c1e47f43b88"

func seqBytes(start byte, count int) []byte {
	out := make([]byte, count)
	for idx := range out {
		out[idx] = start + byte(idx)
	}
	return out
}

func TestX25519DeriveKeyGolden(t *testing.T) {
	curve := ecdh.X25519()
	prvA, err := curve.NewPrivateKey(seqBytes(0x01, 32))
	if err != nil {
		t.Fatal(err)
	}
	prvB, err := curve.NewPrivateKey(seqBytes(0x41, 32))
	if err != nil {
		t.Fatal(err)
	}
	pubA := prvA.PublicKey().Bytes()
	pubB := prvB.PublicKey().Bytes()

	keyAB, err := x25519DeriveKey(prvA.Bytes(), pubB)
	if err != nil {
		t.Fatal(err)
	}
	keyBA, err := x25519DeriveKey(prvB.Bytes(), pubA)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(keyAB, keyBA) {
		t.Fatalf("derivation not symmetric: %x vs %x", keyAB, keyBA)
	}
	if gotHex := hex.EncodeToString(keyAB); gotHex != x25519DeriveGolden {
		t.Fatalf("derived key drifted from golden:\n got  %s\n want %s", gotHex, x25519DeriveGolden)
	}
}
