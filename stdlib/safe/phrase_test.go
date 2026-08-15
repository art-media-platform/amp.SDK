package safe_test

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	_ "github.com/art-media-platform/amp.SDK/stdlib/safe/poly25519"
)

func TestPhrase_Roundtrip(t *testing.T) {
	for _, size := range []int{1, 16, 32, 64} {
		entropy := make([]byte, size)
		if _, err := rand.Read(entropy); err != nil {
			t.Fatal(err)
		}
		phrase := safe.EncodePhrase(entropy)
		if len(phrase) != size+safe.PhraseChecksumSize {
			t.Fatalf("size %d: got %d words, want %d", size, len(phrase), size+safe.PhraseChecksumSize)
		}
		decoded, err := safe.DecodePhrase(phrase)
		if err != nil {
			t.Fatalf("size %d: decode: %v", size, err)
		}
		if !bytes.Equal(decoded, entropy) {
			t.Fatalf("size %d: roundtrip mismatch: got %x want %x", size, decoded, entropy)
		}
	}
}

func TestPhrase_StringParseRoundtrip(t *testing.T) {
	entropy := make([]byte, 16)
	rand.Read(entropy)
	phrase := safe.EncodePhrase(entropy)
	text := phrase.String()
	if strings.Count(text, " ") != len(phrase)-1 {
		t.Fatalf("unexpected separators in %q", text)
	}

	// Case, surrounding whitespace, internal runs — all normalized.
	mangled := "  " + strings.ToUpper(text) + "   \t\n "
	parsed := safe.ParsePhrase(mangled)
	if len(parsed) != len(phrase) {
		t.Fatalf("parse length mismatch: got %d want %d", len(parsed), len(phrase))
	}
	decoded, err := safe.DecodePhrase(parsed)
	if err != nil {
		t.Fatalf("decode after parse: %v", err)
	}
	if !bytes.Equal(decoded, entropy) {
		t.Fatalf("roundtrip mismatch after string/parse")
	}
}

// phraseSwapEntropy is the golden fixture for the word-swap rejection tests.
// A word swap on RANDOM entropy passes the checksum with probability
// 2^(-8·PhraseChecksumSize) — swapping word[0] of this fixed entropy is a
// verified checksum mismatch, so rejection is deterministic.
var phraseSwapEntropy = []byte{
	0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
	0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
}

// phraseWithSwappedWord returns phraseSwapEntropy's phrase with word[0]
// swapped for a different valid word.
func phraseWithSwappedWord(t *testing.T) safe.Phrase {
	t.Helper()
	phrase := safe.EncodePhrase(phraseSwapEntropy)
	swap := safe.PhraseWordAt(1)
	if phrase[0] == swap {
		t.Fatalf("fixture invalid: word[0] %q equals its swap", phrase[0])
	}
	return append(safe.Phrase{swap}, phrase[1:]...)
}

func TestPhrase_ChecksumRejectsWordSwap(t *testing.T) {
	bad := phraseWithSwappedWord(t)
	if _, err := safe.DecodePhrase(bad); err == nil {
		t.Fatal("expected checksum rejection on word swap")
	}
}

// TestPhrase_SwapRejectionRate asserts the widened checksum's rejection rate
// (280 D-phrase-checksum-width): 2000 random single-word swaps must ALL be
// rejected.  At the 4-byte width a false accept is 2⁻³² (expected failures
// here ≈ 5·10⁻⁷); at the old 1-byte width ~8 of 2000 swaps passed
// (P(≥1) ≈ 99.96%), so this test fails against any regression to it.
func TestPhrase_SwapRejectionRate(t *testing.T) {
	entropy := make([]byte, 16)
	for trial := 0; trial < 2000; trial++ {
		if _, err := rand.Read(entropy); err != nil {
			t.Fatal(err)
		}
		phrase := safe.EncodePhrase(entropy)
		pos := trial % len(phrase)
		idx := safe.PhraseWordIndex(phrase[pos])
		swapped := append(safe.Phrase(nil), phrase...)
		swapped[pos] = safe.PhraseWordAt((idx + 1) % safe.PhraseWordCount)
		if _, err := safe.DecodePhrase(swapped); err == nil {
			t.Fatalf("trial %d: single-word swap at %d passed the %d-byte checksum",
				trial, pos, safe.PhraseChecksumSize)
		}
	}
}

func TestPhrase_RejectsUnknownWord(t *testing.T) {
	entropy := make([]byte, 16)
	rand.Read(entropy)
	phrase := safe.EncodePhrase(entropy)
	bad := append(safe.Phrase{"xyzzy"}, phrase[1:]...)
	if _, err := safe.DecodePhrase(bad); err == nil {
		t.Fatal("expected error on unknown word")
	}
}

func TestPhrase_RejectsTooShort(t *testing.T) {
	if _, err := safe.DecodePhrase(safe.Phrase{}); err == nil {
		t.Fatal("expected error on empty phrase")
	}
	if _, err := safe.DecodePhrase(safe.Phrase{"able"}); err == nil {
		t.Fatal("expected error on checksum-only phrase")
	}
}

func TestPhrase_DeriveKeyStable(t *testing.T) {
	entropy := make([]byte, 16)
	rand.Read(entropy)
	phrase := safe.EncodePhrase(entropy)

	key1, err := phrase.DeriveKey("epoch")
	if err != nil {
		t.Fatal(err)
	}
	key2, err := phrase.DeriveKey("epoch")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(key1, key2) {
		t.Fatal("same phrase+purpose must yield identical key")
	}

	// Domain separation — different purpose → different key.
	key3, err := phrase.DeriveKey("founder-sig")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(key1, key3) {
		t.Fatal("different purpose must yield different key")
	}
}

func TestPhrase_KeyPairDeterministic(t *testing.T) {
	entropy := make([]byte, 32)
	rand.Read(entropy)
	phrase := safe.EncodePhrase(entropy)

	spec := safe.KeySpec{
		CryptoKitID: safe.Crypto.Poly25519.ID,
		KeyType:     safe.KeyType_SigningKey,
	}

	kp1, err := safe.KeyPairFromPhrase(phrase, spec, "founder-sig")
	if err != nil {
		t.Fatal(err)
	}
	kp2, err := safe.KeyPairFromPhrase(phrase, spec, "founder-sig")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(kp1.Pub.Bytes, kp2.Pub.Bytes) || !bytes.Equal(kp1.Prv, kp2.Prv) {
		t.Fatal("same phrase+purpose+spec must yield identical KeyPair")
	}

	// Domain separation: different purpose → different key
	kp3, err := safe.KeyPairFromPhrase(phrase, spec, "device-link")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(kp1.Pub.Bytes, kp3.Pub.Bytes) {
		t.Fatal("different purpose must yield different KeyPair")
	}
}

func TestPhrase_KeyPairRejectsBadChecksum(t *testing.T) {
	bad := phraseWithSwappedWord(t)

	spec := safe.KeySpec{
		CryptoKitID: safe.Crypto.Poly25519.ID,
		KeyType:     safe.KeyType_SigningKey,
	}
	if _, err := safe.KeyPairFromPhrase(bad, spec, "founder-sig"); err == nil {
		t.Fatal("expected checksum rejection before key derivation")
	}
}

// Golden phrase vectors — generated ONCE from this package and checked in as
// literals here and in amp-web/src/crypto/phrase.test.ts.  Both suites pin the
// SAME vectors: byte-identical round-trips on both sides are the wordlist's
// freeze contract (SD-did-identity §12.1), so any byte motion in the generated
// PhraseWords const trips both.
const phraseGoldenPurpose = "golden.purpose.v1"

var phraseGolden = []struct {
	seedHex string
	words   string
	keyHex  string // DeriveKey(phraseGoldenPurpose)
}{
	{
		seedHex: "000102030405060708090a0b0c0d0e0f",
		words: "able acid acre agent album alert alien alloy amber anchor angel ankle apple arena armor artist " +
			"storm nickel canyon rose",
		keyHex: "1a9b422a2e49008ffb5f7703b8bc0e1eda0af571549213cf23d8c3ff4d5f6a4e",
	},
	{
		seedHex: "9f8e7d6c5b4a39281706f5e4d3c2b1a0ff00eeddccbbaa998877665544332211",
		words: "harbor focus echo dart cloud camel brass black bacon alien torch silver quiet ocean ladder harvest " +
			"zebra able stone rust petal meadow horn golden field drift crate chime bulb bone bear atom " +
			"angel green blade laser",
		keyHex: "c33407ee95831f951c4fed138bd5b1bd89ada41b4487d4a67dfa7bd819ab0670",
	},
}

// phraseGolden[0]'s phrase with word[0] swapped for wordlist[1] — a verified
// checksum mismatch, so rejection is deterministic on both sides.
const phraseGoldenReject = "acid acid acre agent album alert alien alloy amber anchor angel ankle apple arena armor artist " +
	"storm nickel canyon rose"

func TestPhrase_GoldenVectors(t *testing.T) {
	for gi, golden := range phraseGolden {
		seed, err := hex.DecodeString(golden.seedHex)
		if err != nil {
			t.Fatal(err)
		}
		phrase := safe.EncodePhrase(seed)
		if phrase.String() != golden.words {
			t.Fatalf("vector %d: encode mismatch:\n got %q\nwant %q", gi, phrase.String(), golden.words)
		}
		decoded, err := safe.DecodePhrase(safe.ParsePhrase(golden.words))
		if err != nil {
			t.Fatalf("vector %d: decode: %v", gi, err)
		}
		if !bytes.Equal(decoded, seed) {
			t.Fatalf("vector %d: decode mismatch: got %x want %x", gi, decoded, seed)
		}
		key, err := phrase.DeriveKey(phraseGoldenPurpose)
		if err != nil {
			t.Fatalf("vector %d: derive: %v", gi, err)
		}
		if hex.EncodeToString(key) != golden.keyHex {
			t.Fatalf("vector %d: derived key mismatch: got %x want %s", gi, key, golden.keyHex)
		}
	}
}

func TestPhrase_GoldenChecksumReject(t *testing.T) {
	if _, err := safe.DecodePhrase(safe.ParsePhrase(phraseGoldenReject)); err == nil {
		t.Fatal("expected checksum rejection on the golden word-swap phrase")
	}
}

func TestPhrase_WordlistInvariants(t *testing.T) {
	seen := make(map[string]struct{}, safe.PhraseWordCount)
	for i := range safe.PhraseWordCount {
		word := safe.PhraseWordAt(i)
		if word == "" {
			t.Fatalf("empty word at index %d", i)
		}
		if _, dup := seen[word]; dup {
			t.Fatalf("duplicate word %q at index %d", word, i)
		}
		seen[word] = struct{}{}
		if safe.PhraseWordIndex(word) != i {
			t.Fatalf("index mismatch for %q: got %d want %d", word, safe.PhraseWordIndex(word), i)
		}
	}
	if safe.PhraseWordIndex("NotInList") != -1 {
		t.Fatal("expected -1 for unknown word")
	}
}

// TestPhrase_WordlistPrefixUnique4 mechanically enforces the wordlist
// properties safe.consts.sdl claims (280 D-wordlist-prefix-claims): every
// word is unique on its first FOUR characters (abbreviation entry needs at
// least four typed characters — three-char prefixes are NOT unique),
// 4–7 characters long, and the list is alphabetized.
func TestPhrase_WordlistPrefixUnique4(t *testing.T) {
	prefixes := make(map[string]string, safe.PhraseWordCount)
	prev := ""
	for i := range safe.PhraseWordCount {
		word := safe.PhraseWordAt(i)
		if len(word) < 4 || len(word) > 7 {
			t.Errorf("word %q: length %d outside 4-7", word, len(word))
		}
		if word <= prev {
			t.Errorf("word %q: not alphabetized after %q", word, prev)
		}
		prev = word
		prefix := word
		if len(prefix) > 4 {
			prefix = prefix[:4]
		}
		if other, dup := prefixes[prefix]; dup {
			t.Errorf("4-char prefix collision: %q vs %q", word, other)
		}
		prefixes[prefix] = word
	}
}
