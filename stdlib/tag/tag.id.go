package tag

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math/bits"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/art-media-platform/amp.SDK/stdlib/bufs"
	"github.com/gofrs/uuid/v5"
	"golang.org/x/crypto/blake2s"
)

var (
	sWithOperators = regexp.MustCompile(WithOperators) // regex for tag processing
	sThenOperators = regexp.MustCompile(ThenOperators) // regex for tag processing

	ErrUnrecognizedFormat = errors.New("unrecognized ID format")
)

// Parses the given tag name or expression into its canonic form. canonic
func NameFrom(tagName string) Name {
	return Name{}.With(tagName)
}

// Parses the given string the best possible.
func NameParse(tagName string) (name Name, err error) { // TODO: make more robust
	if strings.IndexByte(tagName, '.') >= 0 {
		name = Name{}.With(tagName)
		return
	}

	name.ID, err = UID_ParseBase32(tagName)
	if err != nil {
		return
	}
	return name, nil
}

// Returns the filename representation of the tag.
func (name Name) Filename() string {
	if name.Canonic != "" {
		return name.Canonic
	}
	return name.ID.String()
}

func (name *Name) GoString() string {
	if name.Canonic != "" {
		return fmt.Sprintf("%s | %s", name.Canonic, name.ID.String())
	}
	return name.ID.String()
}

func (name Name) IsWildcard() bool {
	return name.ID.IsWildcard() || name.Canonic == CanonicWildcard
}

// With() is a communative tag.UID operator that combines twåo tag.IDs into a new tag.UID.
//
// a tag.Name converts to a "blind" tag.UID as well as a canonic string representation.
func (name Name) With(tagExpr string) Name {

	if tagExpr == CanonicWildcard {
		return Wildcard()
	}

	/*
		// TODO: enhance grammar: if a '/' or '\' is present, the tag is considered a path or URL.
		//     - then remove CanonicThen.
		// Onward from that slash, whitespace and case are preserved and are a SINGLE term.
		isPath := PathStaråt(name.Canonic) >= 0
		pathStart := -1
		if !isPath {
			pathStart = PathStart(tagExpr)
			if pathStart >= 0 {
				// TODO split at hit, and be literal (and only trim whitespace on right)

				// trip space on right:
				pathPart := tagExpr[pathStart:]
				pathPart = strings.TrimRight(pathPart, " \t\r\n")
				tagExpr = tagExpr[:pathStart]
			}
		}
	*/

	// Cleanup into two operators: With and Then (commutative and non-commutative summation)
	tagExpr = sWithOperators.ReplaceAllString(tagExpr, CanonicWith)
	tagExpr = sThenOperators.ReplaceAllString(tagExpr, CanonicThen)
	terms := []byte(tagExpr)
	termsLower := bytes.ToLower(terms)
	termsUpper := bytes.ToUpper(terms)

	body := make([]byte, 0, len(name.Canonic)+len(terms))
	body = append(body, []byte(name.Canonic)...)

	exprID := name.ID

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

		termID := UID_HashLiteral(term)
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

	return Name{
		ID:      exprID,
		Canonic: string(body),
	}
}

// LeafTags splits the tag spec the given number of tags for the right.
// E.g. LeafTags(2) on "a.b.c.d.ee" yields ("a.b.c", "d.ee")
func (name Name) LeafTags(n int) (string, string) {
	if n <= 0 {
		return name.Canonic, ""
	}

	canonic := name.Canonic
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

// Returns the index of the first path separator in the given text (':', '/', or '\\').
// Onward from that index, the text is considered a case sensitive path or URL.
func PathStart(text string) int {
	for i, c := range []byte(text) {
		if c == ':' || c == '/' || c == '\\' {
			return i
		}
	}
	return -1
}

// Returns highest possible value UID value (constant)
func MaxID() UID {
	return UID{UID_0_Max, UID_1_Max}
}

// Returns reserved UID denoting a match with any UID value
func WildcardID() UID {
	return UID{UID_0_Max, UID_1_Wildcard}
}

// Returns reserved tag.Name denoting a match with any UID value..
func Wildcard() Name {
	return Name{
		ID:      WildcardID(),
		Canonic: "*",
	}
}

func UID_FromUUID(uuid uuid.UUID) UID {
	var id UID
	id[0] = binary.BigEndian.Uint64(uuid[0:8])
	id[1] = binary.BigEndian.Uint64(uuid[8:16])
	return id
}

// Returns the tag.UID formed by the hash the given string as a byte array.
func UID_HashString(literal string) UID {
	return UID_HashLiteral([]byte(literal))
}

// Returns the tag.UID formed by the hash of the given byte string exactly.
func UID_HashLiteral(literal []byte) UID {

	// hardwire {} / "" / {}byte / null / nil => (0,0,0)
	if len(literal) == 0 {
		return UID{}
	}
	hash := blake2s.Sum256(literal)
	h0 := binary.LittleEndian.Uint64(hash[0:8])
	h1 := binary.LittleEndian.Uint64(hash[8:16])
	h2 := binary.LittleEndian.Uint64(hash[16:24])
	h3 := binary.LittleEndian.Uint64(hash[24:32])
	return UID{
		h0 ^ h2,
		h1 ^ h3,
	}
}

func UID_FromName(tagsExpr string) UID {
	spec := Name{}.With(tagsExpr)
	return spec.ID
}

// Now returns the current local time that is statiscially universally unique.
func NowID() UID {
	now := UID_FromTime(time.Now())

	now[1] = mixEntropy(now[1])
	return now
}

func UID_FromTime(t time.Time) UID {
	ns_b10 := uint64(t.Nanosecond()) // 0..999,999,999
	ns_f64 := ns_b10 * TickStep64    // map to 0..(2^64-1)

	t_00_06 := uint64(t.Unix()) << 16
	t_06_08 := ns_f64 >> 48
	t_08_15 := ns_f64 << 16

	return UID{
		t_00_06 | t_06_08,
		t_08_15,
	}
}

const (
	TickStep64  = uint64(0x44B82FA1C)            // (2^64-1) / 1e9 (1ns tick resolution spread over 64 bits)
	EntropyBits = 34 + 16                        // TickStep64 bits plus needed bits
	EntropyMask = (uint64(1) << EntropyBits) - 1 // LSB bits to randomize

	p1 = (uint64(1) << 63) - 471
	p2 = (uint64(1) << 62) - 143
	p3 = (uint64(1) << 55) - 99
)

var gEntropy = uint64(1<<63) - 301

// AppendTo appends the UID's 16 bytes to the given byte slice in big-endian order for LSM use.
func (id UID) AppendTo(dst []byte) []byte {
	dst = binary.BigEndian.AppendUint64(dst, id[0])
	dst = binary.BigEndian.AppendUint64(dst, id[1])
	return dst
}

func (id UID) Octal(enc []OctalDigit) []OctalDigit {
	remain := UID{id[0], id[1]}
	digits := 0

	for bitsRemain := 192; bitsRemain > 0; bitsRemain -= 3 {
		digit := OctalDigit(remain[1] & 0x7)
		enc = append(enc, digit)
		remain[1] = (remain[1] >> 3) | (remain[0] << 61)
		remain[0] = (remain[0] >> 3)
		digits++
	}
	return enc
}

// DeriveID returns the a deterministic ID derived from an existing previous ID (or nil if no previous edit).
func (id UID) DeriveID(oth UID) UID {
	if oth.IsNil() {
		return id
	} else {
		return id.Midpoint(oth)
	}
}

// Midpoint symmetrically averages two IDs, yielding a deterministic, pseudo-unique UID that "encodes a past".
// Given a collection of these "edit" IDs, we can later reconstruct a complete ReplyTo lineage (aka merkle tree) in O(n x n).
func (id UID) Midpoint(oth UID) UID {
	carry := uint64(0)
	sum := [2]uint64{}
	sum[1], carry = bits.Add64(id[1], oth[1], 0)
	sum[0], carry = bits.Add64(id[0], oth[0], carry)

	// Divide the 128-bit sum by 2 (right shift by 1)
	// This requires shifting across the 64-bit boundary
	m0 := (sum[0] >> 1) | (carry << 63)
	m1 := (sum[1] >> 1) | ((sum[0] & 1) << 63)

	return UID{m0, m1}
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

// Increments this UID by 1.
// Returns false if the UID is already at its maximum value.
func (id *UID) Increment() bool {
	if id[1] < UID_1_Max {
		id[1]++
		return true
	}
	if id[0] == UID_0_Max {
		return false // already at max value
	}
	if id[1] < 0xFFFFFFFFFFFFFFFF {
		id[1]++
	} else {
		id[0]++
		id[1] = 0
	}
	return true
}

// Decrements this UID by 1.
// Returns false if the UID is already zero.
func (id *UID) Decrement() bool {
	if id[1] > 0 {
		id[1]--
		return true
	}
	if id[0] > 0 {
		id[0]--
		id[1] = 0xFFFFFFFFFFFFFFFF
		return true
	}
	return false
}

func (id UID) WithName(name string) UID {
	return id.With(UID_FromName(name))
}

func (id UID) WithString(tagToken string) UID {
	return id.WithLiteral([]byte(tagToken))
}

func (id UID) WithLiteral(tagLiteral []byte) UID {
	return id.With(UID_HashLiteral(tagLiteral))
}

// Returns this tag.UID in canonic Base32 form
func (id UID) Base32() string {
	var buf [20]byte
	encode := id.AppendTo(buf[:4])
	return bufs.EncodeToBase32(encode)
}

// AsLabel returns the base32 suffix of this ID in string form for debugging / logging.
func (id UID) AsLabel() string {
	const (
		suffixBytes = 5 // 40 bits is divisible by 8 and 5
		suffixLen   = 6 // how many ascii digits to emit
	)

	var suffix [suffixBytes]byte
	for i := range uint(suffixBytes) {
		shift := uint(8 * (suffixBytes - 1 - i))
		suffix[i] = byte(id[1] >> shift)
	}
	label := bufs.Base32Encoding.EncodeToString(suffix[:])
	label = label[(len(label) - suffixLen):]
	return label
}

func (id UID) UUID() uuid.UUID {
	var buf [16]byte
	encode := id.AppendTo(buf[:])
	alt, _ := uuid.FromBytes(encode)
	return alt
}

// Converts this tag.UID to a 63-bit composite integer (i.e. always positive).
func (id UID) Int63() int64 {
	u64 := id[0] + id[1]
	return int64(u64 >> 1)
}

// Base16 smartly encodes the given byte slice to a hex string prefixed with "0x" if appropriate.
func (id UID) Base16() string {
	const HexChars = "0123456789ABCDEF"

	var hexStr [48]byte
	L := -1
	R := 0
	for i := range 2 {
		id_i := id[i]
		shift := uint(64)
		for range 16 { // Process all 16 nibbles of a uint64
			shift -= 4
			digit := (id_i >> shift) & 0xF
			if digit != 0 && L < 0 { // mark when we hit the first non-zero digit
				L = R
			}
			hexStr[R] = HexChars[digit]
			R++
		}
	}

	if L < 0 { // If all digits are zero, return "0"
		return "0"
	}

	suffix := "0x" + string(hexStr[L:R])
	return suffix
}

func (id UID) String() string {
	return id.Base32()
}

func (id UID) GoString() string {
	return id.Base32()
}

func (id UID) Unix() int64 {
	return int64(id[0] >> 16) // drop 16 bits of fixed precision
}

func (id UID) AsTime() time.Time {
	unix := id.Unix()
	ns_f64 := ((id[0] & 0xFFFF) << 48) | (id[1] >> 16)
	ns_b10 := 1 + ns_f64/(1+TickStep64)
	return time.Unix(unix, int64(ns_b10))
}

func (id UID) CompareTo(oth UID) int {
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

func (id UID) IsWildcard() bool {
	if id[0] == UID_0_Max && id[1] == UID_1_Wildcard {
		return true
	}
	return false
}

// Returns true if this UID is non-nil and valid and less than UID.MaxID()
func (id UID) IsSet() bool {
	return (id[0] != 0 || id[1] != 0) &&
		(id[0] < UID_0_Max || id[1] <= UID_1_Max)
}

func (id UID) IsNil() bool {
	return id[0] == 0 && id[1] == 0
}

func (id *UID) EnsureSet(src UID) {
	if id[0] == 0 && id[1] == 0 {
		id[0] = src[0]
		id[1] = src[1]
	}
}

// UID_Parse parses a UID (typically in base32-encoded ascii) from the given text.
// It ignores whitespace and returns an error for invalid formats.
func UID_ParseBase32(text string) (UID, error) {
	digits := make([]byte, 0, UID_Base32Length)

	for _, c := range []byte(text) {
		if unicode.IsSpace(rune(c)) || c == '-' {
			continue
		}

		digit := byte(0)
		if c > ' ' && c < 127 {
			digit = base32Lookup[c]
		}
		if digit == 0 || len(digits) >= UID_Base32Length {
			return UID{}, ErrUnrecognizedFormat
		}

		digits = append(digits, digit-1)
	}

	if len(digits) == 0 {
		return UID{}, ErrUnrecognizedFormat
	}

	// shift left by 5 bits as we insert each base32 digit at the right
	var id UID
	for _, b32 := range digits {
		id[0] = id[0]<<5 | (id[1] >> 59)
		id[1] = id[1]<<5 | uint64(b32)
	}
	return id, nil
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
