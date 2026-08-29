package amp

// The did:key fold and its base58btc codec — the ONE authoritative codec site;
// consumers (e.g. app.www's did resolver) delegate here rather than re-implement.

import (
	"encoding/binary"
	"fmt"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// multicodecEd25519Pub is the multicodec registry code for ed25519-pub; its
// unsigned-LEB128 varint (ED 01) prefixes the key bytes inside a did:key.
const multicodecEd25519Pub = 0xED

// base58btcAlphabet is the Bitcoin / multibase 'z' alphabet (no 0, O, I, l).
const base58btcAlphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// base58btcIndex maps an ASCII byte to its alphabet position, or -1.
var base58btcIndex = func() [256]int8 {
	var table [256]int8
	for i := range table {
		table[i] = -1
	}
	for i := range len(base58btcAlphabet) {
		table[base58btcAlphabet[i]] = int8(i)
	}
	return table
}()

// Base58BTCEncode encodes raw as base58btc (the multibase 'z' codec).
// Leading zero bytes encode as '1's.
func Base58BTCEncode(raw []byte) string {
	zeros := 0
	for zeros < len(raw) && raw[zeros] == 0 {
		zeros++
	}
	digits := make([]byte, 0, len(raw)*2)
	for _, inByte := range raw[zeros:] {
		carry := int(inByte)
		for digitIdx := range digits {
			carry += int(digits[digitIdx]) << 8
			digits[digitIdx] = byte(carry % 58)
			carry /= 58
		}
		for carry > 0 {
			digits = append(digits, byte(carry%58))
			carry /= 58
		}
	}
	encoded := make([]byte, 0, zeros+len(digits))
	for range zeros {
		encoded = append(encoded, base58btcAlphabet[0])
	}
	for digitIdx := len(digits) - 1; digitIdx >= 0; digitIdx-- {
		encoded = append(encoded, base58btcAlphabet[digits[digitIdx]])
	}
	return string(encoded)
}

// Base58BTCDecode decodes a base58btc string into bytes — the inverse of
// Base58BTCEncode.  Leading '1's decode to leading zero bytes; an empty
// string decodes to an empty slice.
func Base58BTCDecode(encoded string) ([]byte, error) {
	zeros := 0
	for zeros < len(encoded) && encoded[zeros] == '1' {
		zeros++
	}

	// Big-endian base-256 accumulator, built by Horner's method over base 58.
	decoded := make([]byte, 0, len(encoded)) // upper bound on output length
	for charIdx := zeros; charIdx < len(encoded); charIdx++ {
		digit := base58btcIndex[encoded[charIdx]]
		if digit < 0 {
			return nil, fmt.Errorf("invalid base58 char %q", encoded[charIdx])
		}
		carry := int(digit)
		for byteIdx := 0; byteIdx < len(decoded); byteIdx++ {
			carry += int(decoded[byteIdx]) * 58
			decoded[byteIdx] = byte(carry & 0xff)
			carry >>= 8
		}
		for carry > 0 {
			decoded = append(decoded, byte(carry&0xff))
			carry >>= 8
		}
	}

	// decoded is little-endian; reverse and prepend the leading zeros.
	out := make([]byte, zeros+len(decoded))
	for byteIdx := 0; byteIdx < len(decoded); byteIdx++ {
		out[zeros+byteIdx] = decoded[len(decoded)-1-byteIdx]
	}
	return out, nil
}

// DIDKeyUID returns the MemberID the AOM SD-did-identity.md §2 fold mints for a
// signing key — tag.HashName over the canonical did:key URI ("did:key:z" +
// base58btc(multicodec-varint ‖ pubkey)) — and whether the kit has a did:key
// form at all (Ed25519 via Poly25519 is the shipped form; a kit with no
// did:key encoding reads false).  A re-key verifier uses it structurally:
// when the fold of the key being retired IS the MemberID, the identity cannot
// outlive the key and re-key is refused (AOM SD-did-identity.md §2, §12.1).
func DIDKeyUID(kit safe.CryptoKitID, pubKey []byte) (tag.UID, bool) {
	if kit != safe.Crypto.Poly25519.ID || len(pubKey) != 32 {
		return tag.UID{}, false
	}
	payload := binary.AppendUvarint(make([]byte, 0, 2+len(pubKey)), multicodecEd25519Pub)
	payload = append(payload, pubKey...)
	uri := "did:key:z" + Base58BTCEncode(payload)
	return tag.HashName(uri).ID, true
}
