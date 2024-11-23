package tag

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"regexp"
	"strings"
	"time"

	"github.com/art-media-platform/amp.SDK/stdlib/bufs"
)

var (
	sWithDelimiters = regexp.MustCompile(WithDelimiters) // regex for tag processing
	sThenDelimiters = regexp.MustCompile(ThenDelimiters) // regex for tag processing
)

// Genesis returns a tag.ID that denotes an edit lineage root based on a given seed.
//
//	בְּרֵאשִׁ֖ית בָּרָ֣א אֱלֹהִ֑ים אֵ֥ת הַשָּׁמַ֖יִם וְאֵ֥ת הָאָֽרֶץ
func Genesis(seed ID) ID {
	return [3]uint64{
		seed[0],
		seed[1] >> 32, // 00 00 00 00 helps identify a tag.ID as a genesis seed
		seed[2],
	}
}

// FormEditID combines a predecessor and "seed" tag.ID, yielding a new "EditID" tag.  A collection of EditIDs securely reflect a revision lineage that can be reassembled in O(n*n).
//
// This means a sorted list of CellID + AttrID + ItemID + EditID forms a CRDT, where EditID encompasses "height" as described in https://peerlinks.io/protocol.html
func (predecessor ID) FormEditID(seed ID) ID {
	if predecessor.IsNil() {
		return Genesis(seed)
	} else {
		return [3]uint64{
			((predecessor[0] + seed[0]) >> 1),
			predecessor[1] ^ seed[1],
			predecessor[2] ^ seed[2],
		}
	}
}

const (

	// The "with" delimiter can be thought of as ADD or SUM and combines two terms in a commutative way like addition.
	// A '.' by convention helps visually identify an tag string, it's compatible with domain names, and is already a familiar scoping character.
	CanonicWith     = "."
	CanonicWithChar = byte('.')

	CanonicThen     = "-"
	CanonicThenChar = byte('-')
)

func (id ID) AppendAsOctals(enc []OctalDigit) []OctalDigit {
	remain := ID{id[0], id[1], id[2]}
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

func (id ID) FormAsciiBadge() string {
	encBuf := make([]OctalDigit, 0, 64)
	enc := id.AppendAsOctals(encBuf)

	str := append([]byte{}, gBadge6424.Template...)
	for i, di := range gBadge6424.Dots {
		val := CanonicAsciiAlphabet[0]
		if i < len(enc) {
			val = CanonicAsciiAlphabet[enc[i]]
		}

		// Set the dot value where it belongs in the cloned template
		str[di.CharIndex] = byte(val)
	}

	return string(str)
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

// A tag.Expr produces a tag.ID such that each tag.ID is unique and is independent of its component tag literals.
//
//	e.g. "a.b.cc" == "b.a.cc" == "a.cc.b" != "a.cC.b"
func (expr Expr) With(tagExpr string) Expr {

	// Cleanup into two operators: With and Then (commutative and non-commutative summation)
	tagExpr = sWithDelimiters.ReplaceAllString(tagExpr, CanonicWith)
	tagExpr = sThenDelimiters.ReplaceAllString(tagExpr, CanonicThen)
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

		termID := FromLiteral(term)
		switch op {
		case CanonicWithChar:
			exprID = exprID.With(termID)
		case CanonicThenChar:
			exprID = exprID.Then(termID)
		}

		// {tag_operator}{tag_literal}...
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

const (
	NanosecStep = uint64(0x44B82FA1C) // 1<<64 div 1e9 -- reflects Go's single nanosecond resolution spread over a 64 bits
	EntropyMask = uint64(0x3FFFFFFFF) // entropy bit mask for ID[1] -- slightly smaller than 1 ns resolution
)

func FromLiteral(tagLiteral []byte) ID {
	hasher := sha1.New()
	hasher.Write(tagLiteral)

	var hashBuf [20]byte
	hash := hasher.Sum(hashBuf[:0])

	return ID{
		uint64(binary.LittleEndian.Uint32(hash[0:4])), // 0..3
		binary.LittleEndian.Uint64(hash[4:12]),        // 4..11
		binary.LittleEndian.Uint64(hash[12:20]),       // 12..19
	}
}

func FromExpr(tagsExpr string) ID {
	spec := Expr{}.With(tagsExpr)
	return spec.ID
}

func FromToken(literal string) ID {
	return FromLiteral([]byte(literal))
}

func FromTime(t time.Time, addEntropy bool) ID {
	ns_b10 := uint64(t.Nanosecond())
	ns_f64 := ns_b10 * NanosecStep // map 0..999999999 to 0..(2^64-1)

	t_00_06 := uint64(t.Unix()) << 16
	t_06_08 := ns_f64 >> 48
	t_08_15 := ns_f64 << 16
	tag := ID{
		t_00_06 | uint64(t_06_08),
		t_08_15,
		0, // reserved
	}

	if addEntropy {
		gTagSeed = 377377733*ns_f64 ^ gTagSeed
		tag[1] ^= gTagSeed & EntropyMask
		tag[2] ^= gTagSeed * ns_f64
	}

	return tag
}

func Join(prefixTags, suffixTags string) string {
	if prefixTags == "" {
		return suffixTags
	}
	if suffixTags == "" {
		return prefixTags
	}
	if (prefixTags[len(prefixTags)-1] != '.') && (suffixTags[0] != '.') {
		return prefixTags + "." + suffixTags
	}
	if (prefixTags[len(prefixTags)-1] == '.') && (suffixTags[0] == '.') {
		return prefixTags + suffixTags[1:]
	}
	return prefixTags + suffixTags
}

// Returns the current time as a tag.ID, statistically guaranteed to be unique even when called in rapid succession.
func Now() ID {
	return FromTime(time.Now(), true)
}

func (id ID) IsNil() bool {
	return id[0] == 0 && id[1] == 0 && id[2] == 0
}

func (id ID) IsSet() bool {
	return id[0] != 0 || id[1] != 0 || id[2] != 0
}

func (id ID) IsWildcard() bool {
	return id[0] == 0x1 && id[1] == 0x1 && id[2] == 0x1
}

// This operator is commutative and associative, and is used to generate a new ID from two existing ones.
// Since this is commutative, it is reversible, and means tag literals are order independent.
func (id ID) With(other ID) ID {
	return ID{
		id[0] + other[0],
		id[1] + other[1], // overflow is normal
		id[2] + other[2], // overflow is normal
	}
}

// Entangles this ID with another, producing a new ID -- non-commutative.
func (id ID) Then(other ID) ID {
	return ID{
		id[0] - other[0],
		id[1] - other[1], // overflow is normal
		id[2] - other[2], // overflow is normal
	}
}

func (id ID) WithExpr(expr string) ID {
	return id.With(FromExpr(expr))
}

func (id ID) WithToken(tagToken string) ID {
	return id.WithLiteral([]byte(tagToken))
}

func (id ID) WithLiteral(tagLiteral []byte) ID {
	return id.With(FromLiteral(tagLiteral))
}

func (tag ID) String() string {
	return tag.Base32()
}

func (tag ID) CompareTo(oth ID) int {
	if tag[0] < oth[0] {
		return -1
	}
	if tag[0] > oth[0] {
		return 1
	}
	if tag[1] < oth[1] {
		return -1
	}
	if tag[1] > oth[1] {
		return 1
	}
	if tag[2] < oth[2] {
		return -1
	}
	if tag[2] > oth[2] {
		return 1
	}
	return 0
}

func (tag ID) Add(oth ID) ID {
	var out ID
	var carry uint64

	sum := tag[2] + oth[2]
	out[2] = sum
	if sum < tag[2] || sum < oth[2] {
		carry = 1
	}

	// no carry for tag[0]
	out[1] = tag[1] + oth[1] + carry
	out[0] = tag[0] + oth[0]
	return out
}

func (tag ID) Sub(oth ID) ID {
	var out ID
	var borrow uint64

	dif := tag[2] - oth[2]
	out[2] = dif
	if tag[2] < oth[2] || dif > tag[2] {
		borrow = 1
	}
	// no borrow for tag[0] -- by convention, first bytes are a signed UTC seconds value with 16 bits of fixed seconds precision
	out[1] = tag[1] - oth[1] - borrow
	out[0] = tag[0] - oth[0]
	return out
}

// Returns Unix UTC time in milliseconds
func (tag ID) UnixMilli() int64 {
	return int64(tag[0]*1000) >> 16
}

// Returns Unix UTC time in seconds
func (tag ID) Unix() int64 {
	return int64(tag[0]) >> 16
}

// Returns this tag.ID in canonic Base32 form
func (tag ID) Base32() string {
	var buf [25]byte // (25 * 8) % 5 == 0
	binary := tag.AppendTo(buf[:1])
	str := bufs.Base32Encoding.EncodeToString(binary)
	str = strings.TrimLeft(str, "0")
	if str != "" {
		return str
	}
	return "0"
}

func (tag ID) Base16() string {
	buf := make([]byte, 0, 48)
	tagBytes := tag.AppendTo(buf)
	str := hex.EncodeToString(tagBytes)
	str = strings.TrimLeft(str, "0")
	if str != "" {
		return str
	}
	return "0"
}

// Base32Suffix returns the last few digits of this TID in string form (for easy reading, logs, etc)
func (tag ID) Base32Suffix() string {
	const lcm_bits = 40 // divisible by 5 (bits) and 8 (bytes).
	const lcm_bytes = lcm_bits / 8

	var suffix [lcm_bytes]byte
	for i := uint(0); i < lcm_bytes; i++ {
		shift := uint(8 * (lcm_bytes - 1 - i))
		suffix[i] = byte(tag[2] >> shift)
	}
	base32 := bufs.Base32Encoding.EncodeToString(suffix[:])
	return base32
}

// Base16Suffix returns the last few digits of this TID in string form (for easy reading, logs, etc)
func (tag ID) Base16Suffix() string {
	const nibbles = 7
	const HexChars = "0123456789abcdef"

	var suffix [nibbles]byte
	for i := uint(0); i < nibbles; i++ {
		shift := uint(4 * (nibbles - 1 - i))
		hex := byte(tag[2]>>shift) & 0xF
		suffix[i] = HexChars[hex]
	}
	base16 := string(suffix[:])
	return base16
}

// // CopyNext copies the given TID and increments it by 1, typically useful for seeking the next entry after a given one.
// func (tag ID) Xor(other ID) ID {
// 	return ID{
// 		tag[0] ^ other[0],
// 		tag[1] ^ other[1],
// 		tag[2] ^ other[2],
// 	}
// }

// Forms an amp.UID explicitly from two uint64 values.
func IntsToID(x0 int64, x1, x2 uint64) ID {
	return ID{
		uint64(x0),
		x1,
		x2,
	}
}

type Key [24]byte

var gTagSeed = uint64(0x3773000000003773)

var (
	Nil = ID{}
)

func FromBytes(in []byte) (tag ID, err error) {
	var buf [24]byte
	startAt := max(0, 24-len(in))
	copy(buf[startAt:], in)

	tag[0] = binary.BigEndian.Uint64(buf[0:8])
	tag[1] = binary.BigEndian.Uint64(buf[8:16])
	tag[2] = binary.BigEndian.Uint64(buf[16:24])
	return tag, nil
}

func (tag ID) AppendTo(dst []byte) []byte {
	dst = binary.BigEndian.AppendUint64(dst, tag[0])
	dst = binary.BigEndian.AppendUint64(dst, tag[1])
	dst = binary.BigEndian.AppendUint64(dst, tag[2])
	return dst
}

func (tag ID) AsKey() Key {
	var key Key
	tag.ToLSM(key[:])
	return key
}

func DecodeLSM(lsm []byte) ID {
	return ID{
		^binary.BigEndian.Uint64(lsm[0:8]), // higher UTC values appear first
		binary.BigEndian.Uint64(lsm[8:16]),
		binary.BigEndian.Uint64(lsm[16:24]),
	}
}

func (tag ID) ToLSM(dst []byte) {
	binary.BigEndian.PutUint64(dst[0:], ^tag[0]) // higher UTC values appear first
	binary.BigEndian.PutUint64(dst[8:], tag[1])
	binary.BigEndian.PutUint64(dst[16:], tag[2])
}
