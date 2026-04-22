package amp

import (
	"encoding/binary"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// AttestationKey identifies an Attestation deterministically by the facts it
// attests to.  Independent observers of the same fact compute the same key,
// so their filings converge on a single ItemID in the planet ledger rather
// than accumulating N duplicate records.
type AttestationKey struct {
	Type         AttestationType
	Subject      tag.UID // member / channel / etc. being attested about
	CitedTxID    tag.UID // TxMsg being cited as evidence
	CitedOpIndex uint32  // which op within the cited TxMsg (0 if the whole TxMsg)
}

// ItemID returns the convergent CRDT ItemID for this key.  Backed by
// safe.HashKit (Blake2s_256 by default) truncated to 16 bytes.
//
// The derivation is canonical byte-for-byte: Type | Subject | CitedTxID | CitedOpIndex,
// each field encoded in big-endian fixed-width form.  Any peer with the same inputs
// produces the same ItemID.
func (key AttestationKey) ItemID() tag.UID {
	kit, _ := safe.NewHashKit(safe.HashKitID_Blake2s_256)
	buf := make([]byte, 0, 4+16+16+4)
	buf = binary.BigEndian.AppendUint32(buf, uint32(key.Type))
	buf = binary.BigEndian.AppendUint64(buf, key.Subject[0])
	buf = binary.BigEndian.AppendUint64(buf, key.Subject[1])
	buf = binary.BigEndian.AppendUint64(buf, key.CitedTxID[0])
	buf = binary.BigEndian.AppendUint64(buf, key.CitedTxID[1])
	buf = binary.BigEndian.AppendUint32(buf, key.CitedOpIndex)

	kit.Hasher.Reset()
	kit.Hasher.Write(buf)
	digest := kit.Hasher.Sum(nil)
	return tag.UID{
		binary.BigEndian.Uint64(digest[0:8]),
		binary.BigEndian.Uint64(digest[8:16]),
	}
}
