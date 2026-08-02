package amp

import (
	"io"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"github.com/art-media-platform/amp.SDK/stdlib/status"
)

// ── SealedValue: ciphertext-at-an-attr ────────────────────────────────────────
//
// A safe.SealedValue is the payload of a sealed attr: a value sealed under a
// subkey of an epoch ContentKey, stored as ordinary CRDT state that only
// keyholders open (SD-security-sync.md §6.4 composition; ZO §3.1 custody).
// Seal and open live here as the one authoritative pair; the element address
// the record lives at binds the AEAD as AAD so a sealed box cannot be
// transplanted onto a different element undetected.

// SealedContentType is the amp.Tag content type carrying a marshaled
// safe.SealedValue.
const SealedContentType = "application/vnd.amp.sealed"

// sealedValuePurpose is the domain-separation label a sealed-attr subkey
// derives under — distinct from the "epoch-link" and tx content domains.
const sealedValuePurpose = "sealed-attr"

// sealedValueAAD returns the element address the record lives at as the box's
// additional authenticated data.  Identity is bytes, never a render.
func sealedValueAAD(addr tag.Address) []byte {
	lsm := addr.ElementLSM()
	return lsm[:]
}

// SealValueBox seals plaintext under a subkey derived from epochKey (a
// container's epoch ContentKey), bound to the element address the record
// will live at.  The box self-describes its wrapping key in EpochKeyStore
// coordinates: (containerID, epochKey.EpochID, epochKey.Role).
func SealValueBox(rand io.Reader, epochKey safe.SymKey, containerID tag.UID, addr tag.Address, contentType string, plaintext []byte) (*safe.SealedValue, error) {
	if epochKey.EpochID.IsNil() || len(epochKey.Bytes) == 0 {
		return nil, status.Code_BadRequest.Error("amp: SealValueBox: epochKey must carry EpochID and key bytes")
	}
	if containerID.IsNil() {
		return nil, status.Code_BadRequest.Error("amp: SealValueBox: missing containerID")
	}
	if contentType == "" || len(plaintext) == 0 {
		return nil, status.Code_BadRequest.Error("amp: SealValueBox: missing contentType or plaintext")
	}
	subKey, err := safe.DeriveSubKey(epochKey.Bytes, sealedValuePurpose)
	if err != nil {
		return nil, err
	}
	defer safe.Zero(subKey)

	nonce, cipherblob, err := safe.SealAEAD(rand, subKey, plaintext, sealedValueAAD(addr))
	if err != nil {
		return nil, err
	}
	return &safe.SealedValue{
		ContainerID_0: containerID[0],
		ContainerID_1: containerID[1],
		EpochID_0:     epochKey.EpochID[0],
		EpochID_1:     epochKey.EpochID[1],
		Role:          epochKey.Role,
		ContentType:   contentType,
		Ciphertext:    append(nonce, cipherblob...),
	}, nil
}

// OpenValueBox unseals box with epochKey — the ContentKey the box names via
// (ContainerID, EpochID, Role) — recomputing the AAD from addr, the element
// address the record was read from.  Caller must Zero the returned plaintext
// when done.
func OpenValueBox(epochKey safe.SymKey, addr tag.Address, box *safe.SealedValue) ([]byte, error) {
	if box == nil || len(box.Ciphertext) < safe.NonceSize {
		return nil, status.Code_BadRequest.Error("amp: OpenValueBox: malformed box")
	}
	if boxEpoch := (tag.UID{box.EpochID_0, box.EpochID_1}); epochKey.EpochID != boxEpoch {
		return nil, status.Code_BadRequest.Error("amp: OpenValueBox: epochKey does not match box EpochID")
	}
	if epochKey.Role != box.Role {
		return nil, status.Code_BadRequest.Error("amp: OpenValueBox: epochKey role does not match box Role")
	}
	subKey, err := safe.DeriveSubKey(epochKey.Bytes, sealedValuePurpose)
	if err != nil {
		return nil, err
	}
	defer safe.Zero(subKey)

	return safe.OpenAEAD(subKey, box.Ciphertext[:safe.NonceSize], box.Ciphertext[safe.NonceSize:], sealedValueAAD(addr))
}
