package safe

import (
	"crypto/hmac"
	"crypto/sha256"
	"strings"

	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"golang.org/x/crypto/hkdf"
)

// PhraseChecksumSize is the number of checksum bytes appended before encoding.
// Each word carries 8 bits (PhraseWordCount=256), so a 1-byte checksum adds one word.
const PhraseChecksumSize = 1

// Phrase is an ordered list of canonical wordlist words that encodes a byte
// sequence plus a trailing checksum. Each word carries 8 bits of data.
//
// A Phrase of N words carries (N - PhraseChecksumSize) bytes of entropy.
// The checksum is the leading PhraseChecksumSize bytes of the default
// HashKit digest of the entropy. Decoding fails if the checksum does not match.
type Phrase []string

// EncodePhrase encodes entropy as a Phrase, appending a checksum.
// The returned phrase has len(entropy)+PhraseChecksumSize words.
func EncodePhrase(entropy []byte) Phrase {
	digest := phraseDigest(entropy)
	out := make(Phrase, 0, len(entropy)+PhraseChecksumSize)
	for _, bite := range entropy {
		out = append(out, PhraseWordAt(int(bite)))
	}
	for pos := range PhraseChecksumSize {
		out = append(out, PhraseWordAt(int(digest[pos])))
	}
	return out
}

// DecodePhrase returns the entropy encoded by words after verifying the checksum.
func DecodePhrase(words Phrase) ([]byte, error) {
	if len(words) <= PhraseChecksumSize {
		return nil, status.Code_BadRequest.Errorf("safe: phrase too short (got %d words, need > %d)", len(words), PhraseChecksumSize)
	}
	raw := make([]byte, len(words))
	defer Zero(raw)
	for pos, word := range words {
		idx := PhraseWordIndex(word)
		if idx < 0 {
			return nil, status.Code_BadRequest.Errorf("safe: unknown phrase word %q", word)
		}
		raw[pos] = byte(idx)
	}
	cut := len(raw) - PhraseChecksumSize
	entropy := raw[:cut]
	supplied := raw[cut:]
	expect := phraseDigest(entropy)[:PhraseChecksumSize]
	if !hmac.Equal(supplied, expect) {
		return nil, status.Code_BadRequest.Error("safe: phrase checksum mismatch")
	}
	out := make([]byte, len(entropy))
	copy(out, entropy)
	return out, nil
}

// String returns the phrase as a single space-separated string.
func (phrase Phrase) String() string {
	return strings.Join(phrase, " ")
}

// ParsePhrase splits a whitespace-separated phrase string into a Phrase.
// Case is normalized; surrounding and internal whitespace runs are collapsed.
func ParsePhrase(input string) Phrase {
	return Phrase(strings.Fields(strings.ToLower(input)))
}

// DeriveKey derives a purpose-specific key from the phrase's entropy using
// the default HKDF helper. The phrase is verified as a side-effect; a bad
// checksum returns an error without producing key material.
func (phrase Phrase) DeriveKey(purpose string) ([]byte, error) {
	entropy, err := DecodePhrase(phrase)
	if err != nil {
		return nil, err
	}
	defer Zero(entropy)
	return DeriveSubKey(entropy, purpose)
}

// phraseDigest returns the default HashKit digest of entropy.
func phraseDigest(entropy []byte) []byte {
	kit, _ := NewHashKit(0)
	kit.Hasher.Write(entropy)
	return kit.Hasher.Sum(nil)
}

// KeyPairFromPhrase deterministically derives a KeyPair from phrase entropy.
// Purpose provides domain separation — the same phrase yields distinct keys
// for different roles (e.g. "founder-sig", "device-link", "epoch-seed").
//
// The phrase's checksum is verified before any derivation occurs. The returned
// KeyPair's private material is fresh and owned by the caller; Zero() it after use.
func KeyPairFromPhrase(phrase Phrase, spec KeySpec, purpose string) (KeyPair, error) {
	kit, err := GetCryptoKit(spec.CryptoKitID)
	if err != nil {
		return KeyPair{}, err
	}
	if kit.GenerateKey == nil {
		return KeyPair{}, status.Code_Unimplemented.Errorf("CryptoKit %s does not support key generation", spec.CryptoKitID.String())
	}
	entropy, err := DecodePhrase(phrase)
	if err != nil {
		return KeyPair{}, err
	}
	defer Zero(entropy)

	rng := hkdf.New(sha256.New, entropy, nil, []byte(purpose))
	kp := KeyPair{
		Pub: PubKey{
			CryptoKitID: spec.CryptoKitID,
			KeyType:     spec.KeyType,
		},
	}
	if err := kit.GenerateKey(rng, spec.RequestedSize, &kp); err != nil {
		return KeyPair{}, err
	}
	return kp, nil
}
