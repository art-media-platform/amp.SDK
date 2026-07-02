// Package encode implements amp-base32: unpadded base32 over the geohash
// alphabet (lower case is the canonic form), used wherever amp renders binary
// as human-readable text — tag.UID strings, invite tokens, file-safe suffixes —
// with a decoder that tolerates hand-transcription noise.
package encode

import (
	"bufio"
	"encoding/base32"
	"io"
	"strings"
)

const (
	// The amp-base32 alphabet: a base32 (5-bit) symbol set also used by geohash,
	// choosing ascii characters that are visually distinct and easy to read.
	//
	// https://en.wikipedia.org/wiki/Geohash
	//
	Base32Alphabet_Upper = "0123456789BCDEFGHJKMNPQRSTUVWXYZ"
	Base32Alphabet_Lower = "0123456789bcdefghjkmnpqrstuvwxyz"
)

var (
	// Base32Encoding encodes and decodes binary buffers using amp's lower-case
	// geohash alphabet with no padding.  Lower case is the canonic form — it
	// matches tag.UID.Base32 — while NewDecoder / FromBase32 accept either case.
	Base32Encoding = base32.NewEncoding(Base32Alphabet_Lower).WithPadding(base32.NoPadding)
)

// NewEncoder returns a WriteCloser that base32-encodes (amp alphabet) every byte
// written to it onto dst.  Close must be called to flush the final 5-bit group.
func NewEncoder(dst io.Writer) io.WriteCloser {
	return base32.NewEncoder(Base32Encoding, dst)
}

// NewDecoder returns a Reader that decodes amp-base32 text read from src.  It is
// forgiving of the benign noise a token accumulates in transit: ASCII
// whitespace, '-' grouping separators, and a leading UTF-8 byte-order mark are
// ignored; either case is accepted; and the alphabet's omitted look-alikes fold
// to the character they are mistaken for — i, I, l, L → 1 and o, O → 0 — so a
// hand-transcribed token still decodes.  Any other out-of-alphabet character is
// left for the base32 decoder to reject, so a corrupted or wrong-alphabet
// stream fails cleanly instead of decoding to plausible-but-wrong bytes.
func NewDecoder(src io.Reader) io.Reader {
	return base32.NewDecoder(Base32Encoding, &sanitizer{src: bufio.NewReader(src), atStart: true})
}

// ToBase32 encodes a raw byte string to an amp-base32 string that sorts
// lexicographically in ASCII space.  The empty and all-zero inputs collapse to
// the single sentinel "0".
func ToBase32(in []byte) string {
	if len(in) == 0 {
		return "0"
	}
	var out strings.Builder
	encoder := NewEncoder(&out)
	_, _ = encoder.Write(in) // writing to a strings.Builder cannot fail
	_ = encoder.Close()
	str := out.String()
	if strings.Trim(str, "0") == "" {
		return "0"
	}
	return str
}

// FromBase32 decodes an amp-base32 string produced by ToBase32 back to its
// bytes, tolerating the transit noise documented on NewDecoder.  An empty or
// all-noise input decodes to no bytes.
func FromBase32(in string) ([]byte, error) {
	return io.ReadAll(NewDecoder(strings.NewReader(in)))
}

// sanitizer is the io.Reader that strips benign transit noise from a base32
// stream, maps the alphabet's omitted look-alikes to the digit they resemble,
// and folds ASCII letters to the canonic lower-case alphabet — so a token that
// picked up newlines, spaces, dashes, a BOM, case changes, or a hand-transcribed
// i/l-for-1 or o-for-0 still decodes.
type sanitizer struct {
	src     *bufio.Reader
	atStart bool
}

func (san *sanitizer) Read(dst []byte) (int, error) {
	if san.atStart {
		san.atStart = false
		if mark, _ := san.src.Peek(3); len(mark) == 3 &&
			mark[0] == 0xEF && mark[1] == 0xBB && mark[2] == 0xBF {
			_, _ = san.src.Discard(3) // drop a leading UTF-8 BOM
		}
	}
	count := 0
	for count < len(dst) {
		next, err := san.src.ReadByte()
		if err != nil {
			if count > 0 {
				return count, nil
			}
			return 0, err
		}
		switch {
		case next == '-' || next == ' ' || next == '\t' ||
			next == '\r' || next == '\n' || next == '\v' || next == '\f':
			continue // ignore grouping dashes and ASCII whitespace
		case next == 'i' || next == 'I' || next == 'l' || next == 'L':
			next = '1' // omitted from the alphabet; mistaken for one
		case next == 'o' || next == 'O':
			next = '0' // omitted from the alphabet; mistaken for zero
		case next >= 'A' && next <= 'Z':
			next += 'a' - 'A' // fold to the canonic lower alphabet
		}
		dst[count] = next
		count++
	}
	return count, nil
}

// DebugLabel returns a short display form of a byte string for debugging and logging:
// printable ASCII passes through as-is, anything else is base32-encoded;
// input beyond 12 bytes is elided with an ellipsis.
func DebugLabel(in []byte) string {
	if len(in) == 0 {
		return "null"
	}

	buf := in

	const limit = 12
	alreadyASCII := true
	for _, b := range buf {
		if b < 32 || b > 126 {
			alreadyASCII = false
			break
		}
	}

	suffix := ""
	if len(buf) > limit {
		buf = buf[:limit]
		suffix = "…"
	}

	outStr := ""
	if alreadyASCII {
		outStr = string(buf)
	} else {
		outStr = Base32Encoding.EncodeToString(buf)
	}

	return outStr + suffix
}
