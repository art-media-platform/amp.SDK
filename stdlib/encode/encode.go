package encode

import "encoding/base32"

const (
	// Base32Alphabet is a base32 (5-bit) based symbol set also used by geohash.
	// It chooses ascii chatacters that are visually distinct and easy to read.
	//
	// https://en.wikipedia.org/wiki/Geohash
	//
	Base32Alphabet_Upper = "0123456789BCDEFGHJKMNPQRSTUVWXYZ"
	Base32Alphabet_Lower = "0123456789bcdefghjkmnpqrstuvwxyz"

	// IDEA: GeoH3Alphabet = "0123456_"  just use hex except _ also maps to ffffff..
	// TODO: 7 sub hexes correspond to outward, larger hexes (1-2 resloutions higher)
)

var (
	// Base32Encoding is used to encode/decode binary buffer to/from base 32
	Base32Encoding = base32.NewEncoding(Base32Alphabet_Upper).WithPadding(base32.NoPadding)
)

// ToBase32 encodes raw byte string to a base32 string that sorts lexicographically in ASCII space.
func ToBase32(in []byte) string {
	if len(in) == 0 {
		return "0"
	}
	str := Base32Encoding.EncodeToString(in)
	start := -1
	for i := 0; i < len(str); i++ {
		if str[i] != '0' {
			start = i
			break
		}
	}
	if start < 0 {
		return "0"
	}
	return str
}

// DebugLabel returns a base32 encoding of a binary string, limiting it to a short number of character for debugging and logging.
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
