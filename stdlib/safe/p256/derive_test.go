package p256

import (
	"bytes"
	"crypto/ecdh"
	"encoding/hex"
	"testing"
)

// Golden derived-key bytes for fixed keypairs.  Pins the full ECDH→HKDF
// derivation (shared secret + canonical-ordered pubkey info) so any change to
// the derivation path is a test failure, not a silent interop break.
const p256DeriveGolden = "1d0f5b6f0092b789a8e5f72dde9d54f9086db7828bb191c621f7589de2c2504f"

func seqBytes(start byte, count int) []byte {
	out := make([]byte, count)
	for idx := range out {
		out[idx] = start + byte(idx)
	}
	return out
}

func TestP256DeriveKeyGolden(t *testing.T) {
	curve := ecdh.P256()
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

	keyAB, err := ecdhDeriveKey(prvA.Bytes(), pubB)
	if err != nil {
		t.Fatal(err)
	}
	keyBA, err := ecdhDeriveKey(prvB.Bytes(), pubA)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(keyAB, keyBA) {
		t.Fatalf("derivation not symmetric: %x vs %x", keyAB, keyBA)
	}
	if gotHex := hex.EncodeToString(keyAB); gotHex != p256DeriveGolden {
		t.Fatalf("derived key drifted from golden:\n got  %s\n want %s", gotHex, p256DeriveGolden)
	}
}
