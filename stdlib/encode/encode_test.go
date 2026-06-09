package encode

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// bom is the UTF-8 byte-order mark, a common copy-paste artifact.  Built from
// raw bytes so the source file itself carries no BOM.
var bom = string([]byte{0xEF, 0xBB, 0xBF})

// sample builds a deterministic byte slice of length n (no rand → stable goldens).
func sample(n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = byte((i*7 + 13) % 251)
	}
	return out
}

func TestBase32RoundTrip(t *testing.T) {
	for _, n := range []int{1, 2, 3, 4, 5, 7, 8, 16, 31, 32, 49, 89, 320, 1000} {
		in := sample(n)
		text := ToBase32(in)
		got, err := FromBase32(text)
		if err != nil {
			t.Fatalf("len=%d: FromBase32 error: %v", n, err)
		}
		if !bytes.Equal(got, in) {
			t.Fatalf("len=%d: round-trip mismatch\n in=%x\nout=%x", n, in, got)
		}
	}
}

func TestToBase32IsLowerCanonical(t *testing.T) {
	text := ToBase32(sample(64))
	for _, char := range text {
		if !strings.ContainsRune(Base32Alphabet_Lower, char) {
			t.Fatalf("char %q is outside the canonic lower alphabet; got %q", char, text)
		}
	}
}

func TestFromBase32ToleratesTransitNoise(t *testing.T) {
	in := sample(200)
	canonical := ToBase32(in)

	// every mangling a token realistically suffers between issue and accept
	manglings := map[string]string{
		"trailing newline":   canonical + "\n",
		"leading spaces":     "   " + canonical,
		"wrapped at 40":      wrap(canonical, 40),
		"crlf wrapped":       strings.ReplaceAll(wrap(canonical, 64), "\n", "\r\n"),
		"tabs + spaces":      "\t" + canonical[:20] + "  " + canonical[20:] + "\t",
		"dash grouped":       group(canonical, 8, "-"),
		"upper-cased":        strings.ToUpper(canonical),
		"mixed case":         strings.ToUpper(canonical[:30]) + canonical[30:],
		"utf8 BOM prefix":    bom + canonical,
		"everything at once": bom + "  " + group(strings.ToUpper(wrap(canonical, 40)), 4, "-") + "  \n",
	}
	for name, mangled := range manglings {
		got, err := FromBase32(mangled)
		if err != nil {
			t.Errorf("%s: FromBase32 error: %v", name, err)
			continue
		}
		if !bytes.Equal(got, in) {
			t.Errorf("%s: decoded to wrong bytes", name)
		}
	}
}

func TestFromBase32RejectsForeignAlphabet(t *testing.T) {
	// the canonical base64 markers must always be rejected
	if _, err := FromBase32("token+with/markers="); err == nil {
		t.Fatal("base64 markers (+,/,=) must be rejected by the geohash alphabet")
	}
	// letters the geohash alphabet omits (i, l, o, u) and punctuation are foreign
	for _, str := range []string{"hello", "validish@token", "AAAA===="} {
		if _, err := FromBase32(str); err == nil {
			t.Errorf("expected %q to be rejected", str)
		}
	}
}

func TestStreamingMatchesStringWrappers(t *testing.T) {
	in := sample(257)

	var encoded bytes.Buffer
	enc := NewEncoder(&encoded)
	if _, err := enc.Write(in); err != nil {
		t.Fatalf("encoder write: %v", err)
	}
	if err := enc.Close(); err != nil {
		t.Fatalf("encoder close: %v", err)
	}
	if encoded.String() != ToBase32(in) {
		t.Fatal("NewEncoder output disagrees with ToBase32")
	}

	decoded, err := io.ReadAll(NewDecoder(strings.NewReader(encoded.String())))
	if err != nil {
		t.Fatalf("decoder read: %v", err)
	}
	if !bytes.Equal(decoded, in) {
		t.Fatal("NewDecoder output disagrees with input")
	}
}

func TestToBase32ZeroCases(t *testing.T) {
	if got := ToBase32(nil); got != "0" {
		t.Errorf("nil → %q, want \"0\"", got)
	}
	if got := ToBase32([]byte{}); got != "0" {
		t.Errorf("empty → %q, want \"0\"", got)
	}
	if got := ToBase32([]byte{0, 0, 0, 0, 0}); got != "0" {
		t.Errorf("all-zero → %q, want \"0\"", got)
	}
}

// wrap inserts a '\n' every width characters.
func wrap(str string, width int) string {
	var out strings.Builder
	for i := 0; i < len(str); i += width {
		end := min(i+width, len(str))
		if i > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(str[i:end])
	}
	return out.String()
}

// group joins runs of size n with sep (e.g. "wxyz-2pwv-...").
func group(str string, size int, sep string) string {
	var out strings.Builder
	for i := 0; i < len(str); i += size {
		end := min(i+size, len(str))
		if i > 0 {
			out.WriteString(sep)
		}
		out.WriteString(str[i:end])
	}
	return out.String()
}
