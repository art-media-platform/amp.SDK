package safe_test

import (
	"bytes"
	"crypto/rand"
	"strings"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
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

func TestPhrase_ChecksumRejectsWordSwap(t *testing.T) {
	entropy := make([]byte, 16)
	rand.Read(entropy)
	phrase := safe.EncodePhrase(entropy)

	// Swap word[0] for a different valid word.
	original := phrase[0]
	swap := safe.PhraseWordAt(0)
	if original == swap {
		swap = safe.PhraseWordAt(1)
	}
	bad := append(safe.Phrase{swap}, phrase[1:]...)

	if _, err := safe.DecodePhrase(bad); err == nil {
		t.Fatal("expected checksum rejection on word swap")
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
