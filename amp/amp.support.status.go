package amp

import (
	"encoding/binary"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// ── Founder status quorum: co-signed status records for a genesis founder ────
//
// Founder authority is genesis-rooted, so a status record whose Item is a
// genesis founder and whose author is not that founder carries the founder
// quorum's co-signatures over the digest below (MemberEpoch.StatusQuorum);
// the ACC rule and the vault pre-pass verify it at this one site
// (SD-channel-governance §3.2, SD-security-sync §15.4).

// StatusQuorumDigest returns the domain-separated digest each
// MemberEpoch.StatusQuorum CoSignature signs — SigningDomain_MemberStatus
// over (planetID, memberID, Status, editID), each part u32-length-framed by
// SigningParts, under the planet's hash policy.  editID is the record's op
// EditID (== its carrier's TxID): binding it is the replay guard — a quorum
// co-signed over one status record cannot be re-attached to a later record
// after a reinstatement.  Recomputed from fields, never from the record's
// value bytes, which carry the co-signatures themselves.
func StatusQuorumDigest(planetID tag.UID, hashKit safe.HashKitID, memberID tag.UID, memberStatus MemberStatus, editID tag.UID) ([]byte, error) {
	if planetID.IsNil() {
		return nil, status.Code_BadRequest.Error("amp: status quorum digest needs a planet")
	}
	if memberID.IsNil() {
		return nil, status.Code_BadRequest.Error("amp: status quorum digest names no member")
	}
	if editID.IsNil() {
		return nil, status.Code_BadRequest.Error("amp: status quorum digest needs the record's EditID")
	}
	statusPart := binary.BigEndian.AppendUint32(nil, uint32(memberStatus))
	return safe.SigningDigest(hashKit, safe.SigningDomain_MemberStatus,
		planetID.AppendTo(nil),
		memberID.AppendTo(nil),
		statusPart,
		editID.AppendTo(nil),
	)
}

// VerifyStatusQuorum verifies a founder-target status record's quorum: every
// StatusQuorum CoSignature over StatusQuorumDigest(planetID, hashKit,
// MemberTag, Status, editID) against founderKeys with the genesis-frozen
// threshold (required <= 0 = all founders).  A status record carries no
// SigningKey, EncryptKey, WrappedKeys or ReKey; one that does is refused.
// Returns the verified signer IDs.
func (me *MemberEpoch) VerifyStatusQuorum(planetID tag.UID, hashKit safe.HashKitID, founderKeys map[tag.UID]safe.PubKey, required int, editID tag.UID) ([]tag.UID, error) {
	if me == nil || len(me.StatusQuorum) == 0 {
		return nil, status.Code_BadRequest.Error("amp: not a quorum status record (no StatusQuorum CoSignatures)")
	}
	if me.SigningKey != nil || me.EncryptKey != nil || len(me.WrappedKeys) > 0 || len(me.ReKey) > 0 {
		return nil, status.Code_BadRequest.Error("amp: a quorum status record carries keys")
	}
	if me.MemberTag == nil {
		return nil, status.Code_BadRequest.Error("amp: status record names no member")
	}
	digest, err := StatusQuorumDigest(planetID, hashKit, me.MemberTag.UID(), me.Status, editID)
	if err != nil {
		return nil, err
	}
	return VerifyCoSignatureQuorum(me.StatusQuorum, digest, founderKeys, required)
}
