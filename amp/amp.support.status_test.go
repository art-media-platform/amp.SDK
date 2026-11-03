package amp_test

import (
	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// statusRecord builds a founder-target status record co-signed by signers
// over StatusQuorumDigest(planet, member, memberStatus, editID).
func statusRecord(t *testing.T, planetID, memberID, editID tag.UID, state amp.MemberStatus, signers map[tag.UID][]byte) *amp.MemberEpoch {
	t.Helper()
	record := &amp.MemberEpoch{
		MemberTag: amp.TagFromUID(memberID),
		Node:      amp.TagFromUID(amp.HeadNodeID),
		Status:    state,
	}
	digest, err := amp.StatusQuorumDigest(safe.HashKitID_Blake2s_256, planetID, memberID, editID, state)
	if err != nil {
		t.Fatal(err)
	}
	for signerID, prv := range signers {
		record.StatusQuorum = append(record.StatusQuorum, &amp.CoSignature{
			MemberTag: amp.TagFromUID(signerID),
			Signature: signDigest(t, digest, prv),
		})
	}
	return record
}

func TestVerifyStatusQuorum(t *testing.T) {
	planetID := tag.NewID()
	founderA, pubA, prvA := newSigner(t)
	founderB, pubB, prvB := newSigner(t)
	founders := map[tag.UID]safe.PubKey{founderA: pubA, founderB: pubB}
	editID := tag.NewID()
	kit := safe.HashKitID_Blake2s_256
	check := func(planet, edit tag.UID, quorum int) amp.StatusQuorumCheck {
		return amp.StatusQuorumCheck{
			HashKit:  kit,
			PlanetID: planet,
			EditID:   edit,
			Founders: founders,
			Quorum:   quorum,
		}
	}

	// Both founders co-sign a suspension of founder A: verifies at 0 = all.
	suspended := statusRecord(t, planetID, founderA, editID, amp.MemberStatus_Suspended,
		map[tag.UID][]byte{founderA: prvA, founderB: prvB})
	verified, err := suspended.VerifyStatusQuorum(check(planetID, editID, 0))
	if err != nil || len(verified) != 2 {
		t.Fatalf("quorum refused: verified=%v err=%v", verified, err)
	}

	// One signer short of all founders is refused; at threshold 1 it passes.
	oneSigner := statusRecord(t, planetID, founderA, editID, amp.MemberStatus_Suspended,
		map[tag.UID][]byte{founderB: prvB})
	if _, err := oneSigner.VerifyStatusQuorum(check(planetID, editID, 0)); err == nil {
		t.Error("one of two founders passed as all")
	}
	if _, err := oneSigner.VerifyStatusQuorum(check(planetID, editID, 1)); err != nil {
		t.Errorf("threshold 1 refused: %v", err)
	}

	// Replay guard: the same co-signatures under another record's EditID fail.
	if _, err := suspended.VerifyStatusQuorum(check(planetID, tag.NewID(), 0)); err == nil {
		t.Error("a quorum re-attached to another EditID verified")
	}
	// Status is bound: the suspension's signatures do not reinstate.
	suspended.Status = amp.MemberStatus_Active
	if _, err := suspended.VerifyStatusQuorum(check(planetID, editID, 0)); err == nil {
		t.Error("a suspension quorum verified as Active")
	}
	suspended.Status = amp.MemberStatus_Suspended
	// Planet and member are bound.
	if _, err := suspended.VerifyStatusQuorum(check(tag.NewID(), editID, 0)); err == nil {
		t.Error("the quorum verified on another planet")
	}
	suspended.MemberTag = amp.TagFromUID(founderB)
	if _, err := suspended.VerifyStatusQuorum(check(planetID, editID, 0)); err == nil {
		t.Error("the quorum verified for another member")
	}
	suspended.MemberTag = amp.TagFromUID(founderA)

	// A status record carries no keys.
	suspended.SigningKey = signingKeyRef(pubA.Bytes)
	if _, err := suspended.VerifyStatusQuorum(check(planetID, editID, 0)); err == nil {
		t.Error("a quorum record carrying a SigningKey verified")
	}
	suspended.SigningKey = nil

	// An outsider's co-signature names no founder key.
	outsider, _, prvX := newSigner(t)
	forged := statusRecord(t, planetID, founderA, editID, amp.MemberStatus_Suspended,
		map[tag.UID][]byte{outsider: prvX})
	if _, err := forged.VerifyStatusQuorum(check(planetID, editID, 1)); err == nil {
		t.Error("an outsider's co-signature verified")
	}

	// A record without StatusQuorum is not a quorum record.
	if _, err := (&amp.MemberEpoch{MemberTag: amp.TagFromUID(founderA)}).VerifyStatusQuorum(check(planetID, editID, 0)); err == nil {
		t.Error("a record without StatusQuorum verified")
	}
}

func TestStatusQuorumDigest_Inputs(t *testing.T) {
	kit := safe.HashKitID_Blake2s_256
	planetID, memberID, editID := tag.NewID(), tag.NewID(), tag.NewID()
	if _, err := amp.StatusQuorumDigest(kit, tag.UID{}, memberID, editID, amp.MemberStatus_Active); err == nil {
		t.Error("nil planet accepted")
	}
	if _, err := amp.StatusQuorumDigest(kit, planetID, tag.UID{}, editID, amp.MemberStatus_Active); err == nil {
		t.Error("nil member accepted")
	}
	if _, err := amp.StatusQuorumDigest(kit, planetID, memberID, tag.UID{}, amp.MemberStatus_Active); err == nil {
		t.Error("nil EditID accepted")
	}
	// The status quorum domain is distinct from the re-key domain over the
	// same parts (AllSigningDomains covers the cross-rejection).
	if safe.SigningDomain_MemberStatus == safe.SigningDomain_MemberReKey {
		t.Error("status and re-key domains collide")
	}
}
