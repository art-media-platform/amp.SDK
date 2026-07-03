package tag

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math/bits"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/art-media-platform/amp.SDK/stdlib/encode"
	"golang.org/x/crypto/blake2s"
)

var (
	sSeparatorRegex = regexp.MustCompile(SeparatorRegex) // regex for tag processing
)

// Parse resolves any string into a Name, auto-detecting the format.  Use
// this for ANY string that came from outside the program (wire, file,
// CLI flag, user input) so an inbound base32 UID round-trips intact
// instead of being re-hashed.
//
// See the package README for the Parse vs HashName distinction and the
// silent-failure footgun if you pick wrong.  Mirrors C# TagName.Parse.
func Parse(s string) (Name, error) {
	if strings.IndexByte(s, CanonicSeparatorChar) >= 0 || PathStart(s) >= 0 {
		return Name{}.With(s), nil
	}
	if id, err := UID_ParseBase32(s); err == nil {
		return Name{ID: id}, nil
	}
	return Name{}.With(s), nil
}

// ParseUID is Parse followed by .ID — for callers that only want the UID
// and never the canonic string.
func ParseUID(s string) UID {
	name, _ := Parse(s)
	return name.ID
}

// HashName HASHES its input: it canonizes s as a tag-name expression and
// ALWAYS hashes — even a 26-char base32 string the caller may have meant as
// a UID.  Parsing an inbound base32 UID with it silently mints a different
// UID.  Use only on hardcoded canonic expressions you constructed yourself
// (`"eth:" + addr`, `"amp.member.profile"`); wire input uses Parse /
// UID_ParseBase32.  See the package README.
func HashName(s string) Name {
	return Name{}.With(s)
}

// Returns the filename representation of the tag.
func (name Name) Filename() string {
	if name.Text != "" {
		return name.Text
	}
	return name.ID.String()
}

func (name *Name) GoString() string {
	if name.Text != "" {
		return fmt.Sprintf("%s | %s", name.Text, name.ID.String())
	}
	return name.ID.String()
}

// AsLabel returns a compact label for logging / debugging — the human Text when
// present (elided to 32 runes), else the compact "first1…last4" base32 form of
// the UID.
func (name Name) AsLabel() string {
	if name.Text == "" {
		return name.ID.AsLabel()
	}
	return Elide(name.Text, 32)
}

// Elide caps s at maxRunes on rune boundaries, folding the middle into a single
// '…' (leading half, ellipsis, trailing remainder) — the one display trim label
// call sites share.
func Elide(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes || maxRunes < 2 {
		return s
	}
	head := maxRunes / 2
	tail := maxRunes - head - 1
	return string(runes[:head]) + "…" + string(runes[len(runes)-tail:])
}

func (name Name) IsWildcard() bool {
	return name.ID.IsWildcard() || name.Text == CanonicWildcard
}

// Canonic recomputes and returns the folded canonic string — canonize(Text),
// allocating on every call.  Text is the zero-cost stored form and the DEFAULT
// for display, logging, and identifiers (identity is the UID; a string
// re-parsed downstream re-folds to the same UID, so pre-folding it is wasted
// work).  Call Canonic only when the folded bytes are the contract: normalizing
// arbitrary input to canonic form, returning a resolve-style canonic result, or
// a case-insensitive match against a string whose case you do not control.
func (name Name) Canonic() string {
	return canonize(name.Text)
}

// With folds expr into this tag.Name, returning a new Name whose Text is the
// case-preserved tag expression and whose ID is the hash of canonize(Text).
//
// Splits expr at the first URL-trigger char (`:`, `/`, `\`) into a name part
// (segmented on punctuation/whitespace into dot-joined words) and a URL part
// (verbatim).  Case is preserved in Text; the canonic fold that determines ID
// is applied transiently by canonize at hash time.
//
// See the package README for the complete canonization rules,
// examples, and rationale (ASCII case-fold, URL/name split,
// scheme grammar per RFC 3986 §3.1).
func (name Name) With(expr string) Name {

	expr = strings.TrimSpace(expr)
	if expr == "" {
		return name
	}
	if expr == CanonicWildcard {
		return Wildcard()
	}

	// Split expr into name-part (segmented) and URL-part (verbatim).
	namePart := expr
	urlPart := ""
	if split := PathStart(expr); split >= 0 {
		namePart = expr[:split]
		urlPart = expr[split:]
	}

	var body strings.Builder
	body.Grow(len(name.Text) + len(namePart) + len(urlPart) + 1)
	body.WriteString(name.Text)

	// Scheme-only name part: when the URL trigger is present and the name part
	// matches the RFC 3986 scheme grammar (ALPHA *(ALPHA / DIGIT / "+" / "-"
	// / ".")), preserve it atomically as one segment (its case is folded by
	// canonize at hash time, never here).
	if urlPart != "" && isScheme(namePart) {
		if body.Len() > 0 {
			body.WriteByte(CanonicSeparatorChar)
		}
		body.WriteString(namePart)
	} else {
		namePart = sSeparatorRegex.ReplaceAllString(namePart, string(CanonicSeparatorChar))

		namePartLen := len(namePart)
		for i := 0; i < namePartLen; {

			// skip leading delimiters
			for ; i < namePartLen && namePart[i] == CanonicSeparatorChar; i++ {
			}
			if i >= namePartLen {
				break
			}

			end := strings.IndexByte(namePart[i:], CanonicSeparatorChar)
			if end < 0 {
				end = namePartLen
			} else {
				end += i
			}

			// Words are stored case-preserved; the fold happens in canonize.
			term := namePart[i:end]

			if body.Len() > 0 {
				body.WriteByte(CanonicSeparatorChar)
			}
			body.WriteString(term)

			i = end + 1
		}
	}

	// URL part appended verbatim (the URL-trigger char itself is the
	// delimiter — no '.' inserted).
	if urlPart != "" {
		body.WriteString(urlPart)
	}

	// The name part hashes atomically, so word order is significant (reordered
	// words yield distinct UIDs; no commutative literal fold).  See canonicID.
	text := body.String()
	return Name{
		ID:   canonicID(canonize(text)),
		Text: text,
	}
}

// canonize folds a case-preserved tag expression into its canonic string —
// the string whose atomic hash is the tag UID.  It is the single authority for
// the tag case-fold: foldSegment lowercases each name-part word and any scheme
// atom (RFC 3986 §3.1), and the URL / identifier part is left verbatim.  Input
// is already segmented into dot-joined words (see With), so canonize only
// decides case.
func canonize(text string) string {
	if text == "" {
		return ""
	}

	namePart := text
	urlPart := ""
	if split := PathStart(text); split >= 0 {
		namePart = text[:split]
		urlPart = text[split:]
	}

	var body strings.Builder
	body.Grow(len(text))

	if urlPart != "" && isScheme(namePart) {
		body.WriteString(foldSegment(namePart))
	} else {
		namePartLen := len(namePart)
		for i := 0; i < namePartLen; {
			for ; i < namePartLen && namePart[i] == CanonicSeparatorChar; i++ {
			}
			if i >= namePartLen {
				break
			}
			end := strings.IndexByte(namePart[i:], CanonicSeparatorChar)
			if end < 0 {
				end = namePartLen
			} else {
				end += i
			}
			term := foldSegment(namePart[i:end])
			if body.Len() > 0 {
				body.WriteByte(CanonicSeparatorChar)
			}
			body.WriteString(term)
			i = end + 1
		}
	}

	if urlPart != "" {
		body.WriteString(urlPart)
	}
	return body.String()
}

// foldSegment is the package case-fold, applied to each canonic token (a
// name-part word or a URL scheme atom): ASCII letters A–Z fold to a–z; every
// other byte — each byte of a multibyte UTF-8 rune included — is emitted
// verbatim.  It consults no Unicode case table, so the fold reproduces
// bit-identically across languages and Unicode revisions; non-ASCII runes are
// therefore matched byte-exact (FQDNs are punycoded to ASCII upstream, so
// domains fold fully).  This is the DNS / URI ASCII case-insensitivity rule
// (RFC 4343, RFC 3986 §3.1) and the only case-fold in the package.
func foldSegment(term string) string {
	hasUpper := false
	for i := range len(term) {
		if c := term[i]; c >= 'A' && c <= 'Z' {
			hasUpper = true
			break
		}
	}
	if !hasUpper {
		return term
	}
	folded := []byte(term)
	for i := range folded {
		if c := folded[i]; c >= 'A' && c <= 'Z' {
			folded[i] = c + ('a' - 'A')
		}
	}
	return string(folded)
}

// canonicID derives the UID of an already-canonic tag string.  The name part
// (left of the first URL-trigger char) hashes as one atomic literal, so word
// order within a name is significant.  Any URL / scheme:identifier part (RFC
// 3986, from the trigger onward) hashes separately and combines, keeping
// scheme:identifier identities (eth:, did:, CAIP-10) stable and matching the
// "hash the identifier atomically" rule in the package README.
func canonicID(canonic string) UID {
	if split := PathStart(canonic); split >= 0 {
		nameID := UID_HashLiteral([]byte(canonic[:split]))
		return nameID.With(UID_HashLiteral([]byte(canonic[split:])))
	}
	return UID_HashLiteral([]byte(canonic))
}

// isScheme reports whether s matches the RFC 3986 §3.1 scheme grammar:
// ALPHA *( ALPHA / DIGIT / "+" / "-" / "." ).  Used by With() to detect
// when a name part should be preserved atomically (lowercased only) rather
// than word-folded.
func isScheme(s string) bool {
	if len(s) == 0 {
		return false
	}
	c0 := s[0]
	if !((c0 >= 'a' && c0 <= 'z') || (c0 >= 'A' && c0 <= 'Z')) {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '+', c == '-', c == '.':
		default:
			return false
		}
	}
	return true
}

// LeafTags splits the tag spec the given number of tags for the right.
// E.g. LeafTags(2) on "a.b.c.d.ee" yields ("a.b.c", "d.ee")
func (name Name) LeafTags(n int) (string, string) {
	if n <= 0 {
		return name.Text, ""
	}

	text := name.Text
	R := len(text)
	for p := R - 1; p >= 0; p-- {
		switch c := text[p]; c {
		case CanonicSeparatorChar:
			n--
			if n <= 0 {
				prefix := text[:p]
				if c == CanonicSeparatorChar {
					p++ // omit separator from the leaf
				}
				return prefix, text[p:]
			}
		}
	}
	return "", text
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
		ID:   WildcardID(),
		Text: "*",
	}
}

// UID_HashLiteral returns the tag.UID hash of the literal byte string — no
// canonization, no commutative fold.  Use for opaque-bytes identity
// (content hashes, raw blob digests, fixed-format binary tokens).  For
// human-readable text or scheme:identifier expressions where
// canonization is the point, use [HashName] (or [Parse] for
// outside-the-program inputs).
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

// NowID returns a time-based UID with entropy mixed into the low bits,
// making the result statistically universally unique while preserving
// wall-clock ordering in the high bits.
func NowID() UID {
	uid := UID_FromTime(time.Now())

	prev := gEntropy.Load()
	entropy := rot1*uid[1] ^ prev
	uid[1] ^= entropy & EntropyMask
	gEntropy.Store(rot2 * entropy)

	return uid
}

var gEntropy atomic.Uint64

func init() {
	// Per-process seed so NowID entropy isn't in phase across processes
	// (cross-process uniqueness must not rest on the clock alone).  NowID()
	// itself stays crypto-free.
	var seed [8]byte
	rand.Read(seed[:])
	gEntropy.Store(binary.LittleEndian.Uint64(seed[:]))
}

func NewID() UID {
	var seed [16]byte
	rand.Read(seed[:])
	uid := UID{
		binary.LittleEndian.Uint64(seed[:8]),
		binary.LittleEndian.Uint64(seed[8:]),
	}
	return uid
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

	rot1 = (1 << 63) - 471
	rot2 = (1 << 62) - 143
)

// AppendTo appends the UID's 16 bytes to the given byte slice in big-endian order for LSM use.
func (id UID) AppendTo(dst []byte) []byte {
	dst = binary.BigEndian.AppendUint64(dst, id[0])
	dst = binary.BigEndian.AppendUint64(dst, id[1])
	return dst
}

func (id UID) Octal(enc []OctalDigit) []OctalDigit {
	remain := UID{id[0], id[1]}
	digits := 0

	N := (UID_Bits + 2) / 3 // number of octal digits needed to represent a UID
	for range N {
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

// Commutative, associative UID combine — generates a new ID from two existing
// ones.  Canonization no longer folds NAME literals through this (a name part's
// UID is the atomic hash of its canonic string — order significant); canonicID
// still uses it to combine the name part with a scheme:identifier part, and its
// commutativity there is what keeps scheme:identifier UIDs (eth:, did:) stable.
// Also the UID-arithmetic chaining primitive behind WithName/HashLiteral.
func (id UID) With(other UID) UID {
	return id.Add(other)
}

// Entangles this ID with another, producing a new ID -- non-commutative.
func (id UID) Then(other UID) UID {
	return id.Subtract(other)
}

// Commutative 128-bit modular add.  See [UID.With] for the order-independence
// caveat — this is UID arithmetic, not the tag canonization path.
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

// WithName derives a child UID by atomically hashing this UID's bytes
// followed by the canonized name's UID bytes.  Order-preserving — parent
// then child — and collision-resistant (full hash, no commutative fold).
func (id UID) WithName(name string) UID {
	return id.HashLiteral(UID_FromName(name).AppendTo(nil))
}

// HashString derives a child UID by HASHING this UID + a literal token, atomically.
func (id UID) HashString(tagToken string) UID {
	return id.HashLiteral([]byte(tagToken))
}

// HashLiteral derives a child UID by atomically hashing this UID's 16 bytes
// followed by the literal — order-significant (parent precedes literal) and
// collision-resistant.  Use for hierarchical derivation (parent UID + name →
// child UID), e.g. filesystem item IDs.  No commutative fold.
func (id UID) HashLiteral(tagLiteral []byte) UID {
	buf := id.AppendTo(make([]byte, 0, 2*8+len(tagLiteral)))
	buf = append(buf, tagLiteral...)
	return UID_HashLiteral(buf)
}

// Returns this tag.UID in canonic Base32 text form: 26 lowercase geohash
// digits grouped 6-5-5-5-5 with '-' separators (SD-canonization-spec §1.7).
// Decoding strips '-' and whitespace and accepts either case, so grouping
// carries no identity weight.  The dash after digit 16 marks the NowID
// time/entropy boundary (EntropyBits = 50 = the last 10 digits), so
// same-session IDs read as an identical head and a differing tail.
func (id UID) Base32() string {
	x0 := id[0] // MSB
	x1 := id[1] // LSB
	out := make([]byte, UID_Base32GroupedLength)

	isZero := true
	for i := len(out) - 1; i >= 0; i-- {
		if i > 0 && i%6 == 0 {
			out[i] = '-'
			continue
		}
		b32 := byte(x1) & 0x1F // take the least significant 5 bits
		if b32 != 0 {
			isZero = false
		}
		out[i] = encode.Base32Alphabet_Lower[b32]

		x1 = (x0&0x1F)<<59 | (x1 >> 5)
		x0 >>= 5
	}

	if isZero {
		return "0"
	}

	return string(out)
}

// AsLabel returns a compact "first1…last4" base32 label for debugging / logging.
func (id UID) AsLabel() string {
	full := id.Base32()
	if len(full) <= 8 {
		return full
	}
	return full[:1] + "…" + full[len(full)-4:]
}

// Converts this tag.UID to a 63-bit composite integer (i.e. always positive).
func (id UID) Int63() int64 {
	u64 := id[0] + id[1]
	return int64(u64 >> 1)
}

// Base16 encodes this UID as a "0x"-prefixed hex string with leading zeros
// trimmed; the zero UID encodes as "0" (no prefix).
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

// MarshalJSON encodes a UID as a quoted base32 string.  The zero UID
// marshals as the empty string, NOT JSON null — preserves round-trip
// equality and avoids forcing every wire-side struct field to be a
// pointer.
func (id UID) MarshalJSON() ([]byte, error) {
	if id.IsNil() {
		return []byte(`""`), nil
	}
	out := make([]byte, 0, UID_Base32GroupedLength+2)
	out = append(out, '"')
	out = append(out, id.Base32()...)
	out = append(out, '"')
	return out, nil
}

// UnmarshalJSON decodes a quoted base32 (or canonic name expression) into
// a UID via Parse — so an inbound base32 UID round-trips intact instead of
// being re-hashed by HashName-style canonization.  null and "" both
// decode to the zero UID.
func (id *UID) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		*id = UID{}
		return nil
	}
	if len(b) < 2 || b[0] != '"' || b[len(b)-1] != '"' {
		return fmt.Errorf("tag.UID: expected quoted string, got %q", b)
	}
	s := string(b[1 : len(b)-1])
	if s == "" {
		*id = UID{}
		return nil
	}
	*id = ParseUID(s)
	return nil
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

// IsSet returns true if this UID is non-nil and ≤ MaxID (reserved sentinels excluded).
func (id UID) IsSet() bool {
	return (id[0] != 0 || id[1] != 0) && (id[0] < UID_0_Max || id[1] <= UID_1_Max)
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
	for i, c := range encode.Base32Alphabet_Lower {
		base32Lookup[c] = byte(i + 1)
	}
	for i, c := range encode.Base32Alphabet_Upper {
		base32Lookup[c] = byte(i + 1)
	}
}
