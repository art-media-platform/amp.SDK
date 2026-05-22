package poly25519

import (
	"encoding/hex"
	"testing"
)

// Go ↔ TypeScript interop vectors, shared with
// amp-web/src/crypto/interop.test.ts.  Both suites open the SAME two sealed
// blobs against the SAME fixed X25519 private scalar — one blob produced by
// this Go kit, one by the TS kit.  Opening both on both sides locks the
// seal/open envelope as byte-compatible in both directions.
const (
	interopPrvHex = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"

	goSealedHex = "5d9e6b07e0dcada3d8e474be492e3e016c7c369fa8c3782d3967a08b43c30c75f02277bc4cb038b585c0a7a0eece08196cfe2c301018ad10959de095f2dca80032f6af8cf615346d456482b17f2edcc2809909d1cbaa7c31e6d1f671df60bb259f31331d5f8cebdb66a73cde4dd67273398f6b162bbf3c9514078de5"
	tsSealedHex = "bdcb79760c8bd1ed1ff4c697ee5924a0ac455edcc63c497995174d6783ba6f3b4d6c32dc0fae18c3902644f8425c51671ef20086cd226771a009609de6231c8c19678dc466fc9c3c910efe8fdd3c1aca42503295082365ab828e0a62ba671b97aa8479d441590af302f34735c5e81a21f2767bfb0cb026a828bd2769"

	goPlaintext = "amp Poly25519 interop: sealed by Go, opened anywhere"
	tsPlaintext = "amp Poly25519 interop: sealed by TS, opened anywhere"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	out, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("decode hex: %v", err)
	}
	return out
}

func TestPoly25519Interop(t *testing.T) {
	prv := mustHex(t, interopPrvHex)
	cases := []struct {
		name      string
		sealedHex string
		want      string
	}{
		{"go-sealed", goSealedHex, goPlaintext},
		{"ts-sealed", tsSealedHex, tsPlaintext},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := open(mustHex(t, tc.sealedHex), prv)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			if string(got) != tc.want {
				t.Fatalf("opened %q, want %q", got, tc.want)
			}
		})
	}
}
