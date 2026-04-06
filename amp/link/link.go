// Package link implements amp's two-tier deep link protocol.
//
// Tier 1 (public): structured path with cleartext planet/node/item UIDs.
//
//	https://{org}/link/{planet}/{node}/{item}
//
// Tier 2 (private): opaque encrypted token containing the address tuple,
// sealed with an HKDF-derived subkey of the planet or channel epoch key.
// Intermediaries (SMS, Slack, email) see a random string — zero metadata leakage.
//
//	https://{org}/link/{opaque-token}
//
// Token layout (plaintext):
//
//	version (1 byte) | planetID (16) | nodeID (16) | itemID (16) = 49 bytes
//
// Token layout (ciphertext):
//
//	nonce (24) | AEAD(plaintext) (49 + 16 tag) = 89 bytes → base32 ≈ 143 chars
package link

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/art-media-platform/amp.SDK/stdlib/encode"
	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

const (
	// TokenVersion is the current wire format version.
	TokenVersion byte = 1

	// tokenPlaintextSize = 1 (version) + 3×16 (UIDs) = 49 bytes.
	tokenPlaintextSize = 1 + 3*tag.UID_Size

	// HKDF purpose string for link token encryption.
	hkdfPurpose = "link-token"
)

// LinkToken holds the decoded contents of a deep link.
type LinkToken struct {
	Version   byte
	PlanetID  tag.UID
	NodeID tag.UID
	ItemID    tag.UID
}

// SealToken encrypts a LinkToken under the given epoch key and returns
// a URL-safe base32 string suitable for embedding in a deep link path.
func SealToken(tok LinkToken, epochKey []byte) (string, error) {
	linkKey, err := safe.DeriveSubKey(epochKey, hkdfPurpose)
	if err != nil {
		return "", fmt.Errorf("link: derive key: %w", err)
	}
	defer safe.Zero(linkKey)

	plaintext := marshalToken(tok)

	nonce, cipherblob, err := safe.SealAEAD(rand.Reader, linkKey, plaintext, nil)
	if err != nil {
		return "", fmt.Errorf("link: seal: %w", err)
	}

	// Wire format: nonce || cipherblob
	wire := make([]byte, len(nonce)+len(cipherblob))
	copy(wire, nonce)
	copy(wire[len(nonce):], cipherblob)

	return encode.Base32Encoding.EncodeToString(wire), nil
}

// OpenToken decodes a base32 token string and decrypts it with the given epoch key.
// Returns ErrBadToken if decryption fails (wrong key, corrupted, or tampered).
func OpenToken(encoded string, epochKey []byte) (LinkToken, error) {
	wire, err := encode.Base32Encoding.DecodeString(encoded)
	if err != nil {
		return LinkToken{}, fmt.Errorf("link: decode base32: %w", err)
	}

	if len(wire) < safe.NonceSize+tokenPlaintextSize+16 {
		return LinkToken{}, ErrBadToken
	}

	linkKey, err := safe.DeriveSubKey(epochKey, hkdfPurpose)
	if err != nil {
		return LinkToken{}, fmt.Errorf("link: derive key: %w", err)
	}
	defer safe.Zero(linkKey)

	nonce := wire[:safe.NonceSize]
	cipherblob := wire[safe.NonceSize:]

	plaintext, err := safe.OpenAEAD(linkKey, nonce, cipherblob, nil)
	if err != nil {
		return LinkToken{}, ErrBadToken
	}

	tok, err := unmarshalToken(plaintext)
	if err != nil {
		return LinkToken{}, err
	}
	return tok, nil
}

// IsEncryptedToken returns true if the path segment looks like an encrypted
// link token (single segment, no slashes, plausible base32 length).
func IsEncryptedToken(pathSegment string) bool {
	if strings.ContainsRune(pathSegment, '/') {
		return false
	}
	// Encrypted token base32 length: ceil(89 * 8 / 5) = 143 chars
	return len(pathSegment) >= 120 && len(pathSegment) <= 160
}

// ErrBadToken indicates the token could not be decrypted (wrong key, corrupted, or tampered).
var ErrBadToken = fmt.Errorf("link: bad or unreadable token")

func marshalToken(tok LinkToken) []byte {
	buf := make([]byte, tokenPlaintextSize)
	buf[0] = tok.Version
	off := 1
	binary.BigEndian.PutUint64(buf[off:], tok.PlanetID[0])
	binary.BigEndian.PutUint64(buf[off+8:], tok.PlanetID[1])
	off += tag.UID_Size
	binary.BigEndian.PutUint64(buf[off:], tok.NodeID[0])
	binary.BigEndian.PutUint64(buf[off+8:], tok.NodeID[1])
	off += tag.UID_Size
	binary.BigEndian.PutUint64(buf[off:], tok.ItemID[0])
	binary.BigEndian.PutUint64(buf[off+8:], tok.ItemID[1])
	return buf
}

func unmarshalToken(data []byte) (LinkToken, error) {
	if len(data) < tokenPlaintextSize {
		return LinkToken{}, ErrBadToken
	}
	tok := LinkToken{Version: data[0]}
	off := 1
	tok.PlanetID[0] = binary.BigEndian.Uint64(data[off:])
	tok.PlanetID[1] = binary.BigEndian.Uint64(data[off+8:])
	off += tag.UID_Size
	tok.NodeID[0] = binary.BigEndian.Uint64(data[off:])
	tok.NodeID[1] = binary.BigEndian.Uint64(data[off+8:])
	off += tag.UID_Size
	tok.ItemID[0] = binary.BigEndian.Uint64(data[off:])
	tok.ItemID[1] = binary.BigEndian.Uint64(data[off+8:])
	return tok, nil
}

