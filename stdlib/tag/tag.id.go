package tag

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"math"
	"math/bits"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/art-media-platform/amp.SDK/stdlib/bufs"
)

var (
	sWithOperators = regexp.MustCompile(WithOperators) // regex for tag processing
	sThenOperators = regexp.MustCompile(ThenOperators) // regex for tag processing

	ErrUnrecognizedFormat = errors.New("unrecognized ID format")
)

func (expr Expr) AsID() U3D {
	return U3D{
		0,
		uint64(expr.ID[0]),
		uint64(expr.ID[1]),
	}
}

func ParseExpr(tagExpr string) (Expr, error) {
	return Expr{}.With(tagExpr), nil
}

// LeafTags splits the tag spec the given number of tags for the right.
// E.g. LeafTags(2) on "a.b.c.d.ee" yields ("a.b.c", "d.ee")
func (expr Expr) LeafTags(n int) (string, string) {
	if n <= 0 {
		return expr.Canonic, ""
	}

	canonic := expr.Canonic
	R := len(canonic)
	for p := R - 1; p >= 0; p-- {
		switch c := canonic[p]; c {
		case CanonicThenChar, CanonicWithChar:
			n--
			if n <= 0 {
				prefix := canonic[:p]
				if c == CanonicWithChar {
					p++ // omit canonic with operator
				}
				return prefix, canonic[p:]
			}
		}
	}
	return "", canonic
}

// With() is a communative tag.U3D operator that combines two tag.IDs into a new tag.U3D.
//
// a tag.Expr converts to a "blind" tag.UID as well as a canonic string representation.
func (expr Expr) With(tagExpr string) Expr {

	// Cleanup into two operators: With and Then (commutative and non-commutative summation)
	tagExpr = sWithOperators.ReplaceAllString(tagExpr, CanonicWith)
	tagExpr = sThenOperators.ReplaceAllString(tagExpr, CanonicThen)
	terms := []byte(tagExpr)
	termsLower := bytes.ToLower(terms)
	termsUpper := bytes.ToUpper(terms)

	body := make([]byte, 0, len(expr.Canonic)+len(terms))
	body = append(body, []byte(expr.Canonic)...)

	exprID := expr.ID

	N := len(terms)
	for i := 0; i < N; {
		op := CanonicWithChar

		// extract operator
		for ; i < N; i++ {
			c := terms[i]
			if c == CanonicThenChar {
				op = c
			} else if c != CanonicWithChar {
				break
			}
		}

		// find end of tag literal
		start := i
		lowerCount := 0
		for ; i < N; i++ {
			c := terms[i]
			if c == CanonicWithChar || c == CanonicThenChar {
				break
			}
			if c == termsLower[i] && c != termsUpper[i] {
				lowerCount++
			}
		}
		if i == start {
			continue // skip empty terms
		}

		// lower-case is canonic unless literal is a single character or ALL upper-case
		termLen := i - start
		var term []byte
		if termLen == 1 || lowerCount > 0 {
			term = termsLower[start:i]
		} else {
			term = terms[start:i]
		}

		termID := HashLiteral(term)
		switch op {
		case CanonicWithChar:
			exprID = exprID.With(termID)
		case CanonicThenChar:
			exprID = exprID.Then(termID)
		}

		// ({tag_operator}{tag_literal})...
		if len(body) > 0 || op != CanonicWithChar {
			body = append(body, op)
		}
		body = append(body, term...)
	}

	return Expr{
		ID:      exprID,
		Canonic: string(body),
	}
}

// Returns the tag.UID formed by the hash the given string as a byte array.
func HashString(literal string) UID {
	return HashLiteral([]byte(literal))
}

// Returns the tag.UID formed by the hash of the given byte string exactly.
func HashLiteral(literal []byte) UID {

	// hardwire {} / "" / {}byte / null / nil => (0,0,0)
	if len(literal) == 0 {
		return UID{}
	}
	hasher := sha1.New()
	hasher.Write(literal)
	var hashBuf [24]byte
	hash := hasher.Sum(hashBuf[:0])

	return UID{
		binary.LittleEndian.Uint64(hash[0:8]),
		binary.LittleEndian.Uint64(hash[8:16]),
	}
}

func UID_FromExpr(tagsExpr string) UID {
	spec := Expr{}.With(tagsExpr)
	return spec.ID
}

// Now returns the current local time that is statiscially universally unique.
func Now() UID {
	now := UID_FromTime(time.Now())

	entropy := (p1*now[1] + 0xCCCCAAAACCCCAAAA) ^ (p2 * gEntropy)
	now[1] ^= entropy & EntropyMask
	gEntropy = entropy

	return now
}

func NowID() U3D {
	tid := Now()
	return U3D{0, tid[0], tid[1]}
}

// GenesisEditID returns the initial EditID for a tag.UID.
//
//	בְּרֵאשִׁ֖ית בָּרָ֣א אֱלֹהִ֑ים אֵ֥ת הַשָּׁמַ֖יִם וְאֵ֥ת הָאָֽרֶץ
func GenesisEditID() UID {
	id := Now()
	id[1] &^= GenesisEditClearBits // signals this UID as a genesis edit ID
	return id
}

const (
	NanosecStep = uint64(0x44B82FA1C)            // 1<<64 / 1e9 (nanosecond resolution spread over a 64 bits)
	EntropyBits = 64 - 30 + 16                   // bits beyond 1 ns resolution (30 bits for 1e9 nanoseconds plus 16 fixed shift left)
	EntropyMask = (uint64(1) << EntropyBits) - 1 // bits randomized by Now()
)

func UID_FromTime(t time.Time) UID {
	ns_b10 := uint64(t.Nanosecond()) // 0..999999999
	ns_f64 := ns_b10 * NanosecStep   // map to 0..(2^64-1)

	t_00_06 := uint64(t.Unix()) << 16
	t_06_08 := ns_f64 >> 48
	t_08_15 := ns_f64 << 16

	return UID{
		t_00_06 | t_06_08,
		t_08_15,
	}
}

const (
	p1 = (uint64(1) << 63) - 471
	p2 = (uint64(1) << 62) - 143
	p3 = (uint64(1) << 55) - 99
)

var gEntropy = uint64(1<<63) - 301

func (id *U3D) IsNil() bool {
	return id[0] == 0 && id[1] == 0 && id[2] == 0
}

func (id *U3D) IsSet() bool {
	return id[0] != 0 || id[1] != 0 || id[2] != 0
}

func (id U3D) Wildcard() int {
	if id[0] == U3D_0_Symbol && id[1] == 0 {
		if id[2] == U3D_First {
			return 1
		} else if id[2] == U3D_Last {
			return -1
		}
	}
	return 0
}

func (id U3D) String() string {
	return id.Base32()
}

// CompareTo compares this ID to another ID and returns -1, 0, or 1:
//
//	a >  b => +1
//	a == b =>  0
//	a <  b => -1
func (id *U3D) CompareTo(b *U3D) int {
	if id[0] < b[0] {
		return -1
	}
	if id[0] > b[0] {
		return 1
	}
	if id[1] < b[1] {
		return -1
	}
	if id[1] > b[1] {
		return 1
	}
	if id[2] < b[2] {
		return -1
	}
	if id[2] > b[2] {
		return 1
	}
	return 0
}

func (id U3D) Add(oth U3D) U3D {
	var (
		out   U3D
		carry uint64
	)

	out[2], carry = bits.Add64(id[2], oth[2], 0)
	out[1], carry = bits.Add64(id[1], oth[1], carry)
	out[0] = id[0] + oth[0] + carry
	return out
}

func (id U3D) Subtract(oth U3D) U3D {
	var (
		out    U3D
		borrow uint64
	)

	out[2], borrow = bits.Sub64(id[2], oth[2], 0)
	out[1], borrow = bits.Sub64(id[1], oth[1], borrow)
	out[0] = id[0] - oth[0] - borrow
	return out
}

// Returns this tag.UID in canonic Base32 form
func (id U3D) Base32() string {
	var buf [28]byte               // (24 * 8) % 5 == 0, 24 bytes for 192 bits
	binary := id.AppendTo(buf[:1]) // skip 1 byte to pad with 0s
	str := bufs.Base32Encoding.EncodeToString(binary)
	str = strings.TrimLeft(str, "0") // remove leading zeros
	if str != "" {
		return str
	}
	return "0"
}

// AsLabel returns the base32 suffix of this ID in string form (for debugging, logs, etc)
func (id U3D) AsLabel() string {
	const suffixBytes = 5 // choose 5 because 5*8 == 40 bits, also divisible by 5

	var suffix [suffixBytes]byte
	for i := uint(0); i < suffixBytes; i++ {
		shift := uint(8 * (suffixBytes - 1 - i))
		suffix[i] = byte(id[2] >> shift)
	}
	base32 := bufs.Base32Encoding.EncodeToString(suffix[:])[2:] // 6 ascii digit suffix
	return base32
}

func (id U3D) Base16() string {
	return "0x" + id.Base16Suffix(127)
}

// Base16Suffix returns the requested number of ASCII digits (excluding)
func (id U3D) Base16Suffix(maxLen int) string {
	const HexChars = "0123456789ABCDEF"

	var hexStr [3 * 16]byte // Each uint64 needs up to 16 hex digits
	L := 0
	R := 0
	for i := range 3 {
		id_i := id[i]
		shift := uint(64)
		for range 16 { // Process all 16 nibbles of a uint64
			shift -= 4
			digit := (id_i >> shift) & 0xF
			if digit != 0 && L == 0 { // mark when we hit the first non-zero digit
				L = R
			}
			hexStr[R] = HexChars[digit]
			R++
		}
	}

	if L == 0 { // If all digits are zero, return "0"
		return "0"
	}

	if R-L > maxLen {
		L = R - maxLen
	}

	suffix := string(hexStr[L:R])
	return suffix
}

func (id U3D) AppendTo(dst []byte) []byte {
	dst = binary.BigEndian.AppendUint64(dst, id[0])
	dst = binary.BigEndian.AppendUint64(dst, id[1])
	dst = binary.BigEndian.AppendUint64(dst, id[2])
	return dst
}

func (id U3D) Octal(enc []OctalDigit) []OctalDigit {
	remain := U3D{id[0], id[1], id[2]}
	digits := 0

	for bitsRemain := 192; bitsRemain > 0; bitsRemain -= 3 {
		digit := OctalDigit(remain[2] & 0x7)
		enc = append(enc, digit)
		enc[digits] = digit
		remain[2] = (remain[2] >> 3) | (remain[1] << 61)
		remain[1] = (remain[1] >> 3) | (remain[0] << 61)
		remain[0] = (remain[0] >> 3)
		digits++
	}
	return enc
}

func (id U3D) UID() UID {
	return UID{
		id[0] ^ id[1],
		id[2],
	}
}

// Midpoint symmetrically averages two IDs, yielding a determintic, pseudo-unique UID that "encodes a past".
// Given a collection of these "edit" IDs, we can later reconstruct a complete ReplyTo lineage (aka merkle tree) in O(n x n).
func (id UID) Midpoint(oth UID) UID {
	carry := uint64(0)
	sum := [2]uint64{}
	sum[1], carry = bits.Add64(id[1], oth[1], 0)
	sum[0], carry = bits.Add64(id[0], oth[0], carry)

	// Divide the 128-bit sum by 2 (right shift by 1)
	// This requires shifting across the 64-bit boundary
	return UID{
		(sum[0] >> 1) | (carry << 63),
		(sum[1] >> 1) | ((sum[0] & 1) << 63),
	}
}

// This operator is commutative and associative, and is used to generate a new ID from two existing ones.
// Since this is commutative, it is reversible, and means tag literals are order independent.
func (id UID) With(other UID) UID {
	return id.Add(other)
}

// Entangles this ID with another, producing a new ID -- non-commutative.
func (id UID) Then(other UID) UID {
	return id.Subtract(other)
}

// Since this operator is commutative, tag literals are order independent.
func (id UID) Add(oth UID) (out UID) {
	carry := uint64(0)
	out[1], carry = bits.Add64(id[1], oth[1], 0)
	out[0] = id[0] + oth[0] + carry
	return out
}

func (id UID) Subtract(oth UID) (out UID) {
	borrow := uint64(0)
	out[1], borrow = bits.Sub64(id[1], oth[1], 0)
	out[0] = id[0] - oth[0] - borrow
	return out
}

func (id UID) WithExpr(expr string) UID {
	return id.With(UID_FromExpr(expr))
}

func (id UID) WithString(tagToken string) UID {
	return id.WithLiteral([]byte(tagToken))
}

func (id UID) WithLiteral(tagLiteral []byte) UID {
	return id.With(HashLiteral(tagLiteral))
}

// Converts this a UID to a U3D, putting this UID in the lsb.
func (id UID) U3D() U3D {
	return U3D{
		0,
		id[0],
		id[1],
	}
}

// Returns this tag.UID in canonic Base32 form
func (id UID) Base32() string {
	return id.U3D().Base32()
}

func (id UID) AsLabel() string {
	return id.U3D().AsLabel()
}

func (id UID) Base16() string {
	return id.U3D().Base16()
}

func (id UID) String() string {
	return id.Base32()
}

func (id *UID) Unix() int64 {
	return int64(id[0] >> 16) // drop 16 bits of fixed precision
}

func (id *UID) AsTime() time.Time {
	unix := id.Unix()
	ns_f64 := ((id[0] & 0xFFFF) << 48) | (id[1] >> 16)
	ns_b10 := 1 + ns_f64/(1+NanosecStep)
	return time.Unix(unix, int64(ns_b10))
}

func (id *UID) CompareTo(oth *UID) int {
	if id[0] < oth[0] {
		return -1
	}
	if id[0] > oth[0] {
		return 1
	}
	if id[1] < oth[1] {
		return -1
	}
	if id[1] > oth[1] {
		return 1
	}
	return 0
}

func (id *UID) IsSet() bool {
	return id != nil && (id[0] != 0 || id[1] != 0)
}

func (id *UID) IsNil() bool {
	return id == nil || (id[0] == 0 && id[1] == 0)
}

func U3D_Max() U3D {
	return U3D{U3D_0_Max, math.MaxUint64, math.MaxUint64}
}

// U3D_Parse parses a UID or U3D (typically in base32-encoded ascii) from the given text.
// It ignores whitespace and returns an error for invalid formats.
func U3D_Parse(text string) (U3D, error) {
	const maxDigits = (24*8 + 4) / 5 // 24 bytes = 192 bits, base32 = 5 bits per digit
	digits := make([]byte, 0, maxDigits)

	for _, c := range []byte(text) {
		if unicode.IsSpace(rune(c)) {
			continue
		}

		digit := byte(0)
		if c > ' ' && c < 127 {
			digit = base32Lookup[c]
		}
		if digit == 0 || len(digits) >= maxDigits {
			return U3D{}, ErrUnrecognizedFormat
		}

		digits = append(digits, digit-1)
	}

	if len(digits) == 0 {
		return U3D{}, ErrUnrecognizedFormat
	}

	// shift left by 5 bits as we insert each base32 digit at the right
	var id U3D
	for _, b32 := range digits {
		id[0] = id[0]<<5 | (id[1] >> 59)
		id[1] = id[1]<<5 | (id[2] >> 59)
		id[2] = id[2]<<5 | uint64(b32)
	}
	return id, nil
}

// Returns the tag.U3D formed by 24 bytes (or less) of the given bytes.
//
// If the input is less than 24 bytes, the result is left-padded with zeros.
func U3D_FromBytes(in []byte) (id U3D, err error) {
	var buf [24]byte
	startAt := max(0, 24-len(in))
	copy(buf[startAt:], in)

	id[0] = binary.BigEndian.Uint64(buf[0:8])
	id[1] = binary.BigEndian.Uint64(buf[8:16])
	id[2] = binary.BigEndian.Uint64(buf[16:24])
	return id, nil
}

func UID_Max() UID {
	return UID{math.MaxUint64, math.MaxUint64}
}

func UID_Parse(text string) (UID, error) {
	id, err := U3D_Parse(text)
	if id[0] != 0 {
		return UID{}, ErrUnrecognizedFormat
	}
	return UID{
		id[1],
		id[2],
	}, err

}

var base32Lookup = [127]byte{}

func init() {
	for i, c := range bufs.Base32Alphabet_Lower {
		base32Lookup[c] = byte(i + 1)
	}
	for i, c := range bufs.Base32Alphabet_Upper {
		base32Lookup[c] = byte(i + 1)
	}
}
