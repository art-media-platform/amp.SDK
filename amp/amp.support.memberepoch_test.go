package amp

// amp.support.memberepoch_test.go pins the MemberEpoch per-field custody merge
// from the BINDING's vantage (the merged cache is what consumers read):
// member-owned keys survive issuer restatements in every delivery order, an
// issuer still seeds, status stays issuer-owned, and an authorless (serve-lane)
// record moves the base but never a custody field.  Twin of the vault
// member-key cache pins (amp.planet vault.member_custody_test.go) one layer up.

import (
	"testing"
	"time"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

var testMemberEpochAttr = tag.Name{ID: tag.UID{0x77, 0x1}, Text: "test.MemberEpoch"}

// memberEpochRecord parameterizes one MemberEpoch op: From == Member makes a
// self-record; "" leaves that key undeclared; ServeTxID, when set, re-stamps
// the carrying tx AFTER the op is marshaled, so the op's EditID keeps TxID and
// the tx's TxID moves — the snapshot serve shape (authorless lane).
type memberEpochRecord struct {
	From       tag.UID
	TxID       tag.UID
	ServeTxID  tag.UID
	Member     tag.UID
	SignPub    string
	EncryptPub string
	Status     MemberStatus
}

func memberEpochUpdate(t *testing.T, rec memberEpochRecord) NodeUpdate {
	t.Helper()
	tx := TxNew()
	tx.SetTxID(rec.TxID)
	tx.SetFromID(rec.From)
	epoch := &MemberEpoch{
		MemberTag: TagFromUID(rec.Member),
		Status:    rec.Status,
	}
	if rec.SignPub != "" {
		epoch.SigningKey = &safe.KeyRef{PubKey: []byte(rec.SignPub)}
	}
	if rec.EncryptPub != "" {
		epoch.EncryptKey = &safe.KeyRef{PubKey: []byte(rec.EncryptPub)}
	}
	if err := tx.Upsert(HeadNodeID, testMemberEpochAttr.ID, rec.Member, epoch); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if rec.ServeTxID.IsSet() {
		tx.SetTxID(rec.ServeTxID) // ops keep the original EditID stamp; the tx is now synthetic
	}
	return NodeUpdate{NodeID: HeadNodeID, Revision: tx.TxID(), Tx: tx}
}

func newMemberEpochBinding() *FoldBinding[*MemberEpoch] {
	binding := NewFoldBinding[*MemberEpoch](testMemberEpochAttr)
	binding.Bind(HeadNodeID)
	binding.Merger = NewMemberEpochMerger()
	return binding
}

func mergedKeys(t *testing.T, binding *FoldBinding[*MemberEpoch], member tag.UID) (signPub, encryptPub string, status MemberStatus) {
	t.Helper()
	epoch, ok := binding.GetItem(member)
	if !ok {
		t.Fatalf("member %s not cached", member.AsLabel())
	}
	if epoch.SigningKey != nil {
		signPub = string(epoch.SigningKey.PubKey)
	}
	if epoch.EncryptKey != nil {
		encryptPub = string(epoch.EncryptKey.PubKey)
	}
	return signPub, encryptPub, epoch.Status
}

// TestMemberEpochMerge_IssuerCannotRegressSelfRotated: the WAN-race shape —
// issuer seed, member self-rotation, issuer restatement at a NEWER EditID — in
// both delivery orders.  The merged cache must answer the member's key; a
// plain-LWW binding answers the restatement (the control documenting what the
// merger replaces).
func TestMemberEpochMerge_IssuerCannotRegressSelfRotated(t *testing.T) {
	admin, bob := tag.NewID(), tag.NewID()
	base := time.Now()
	seed := memberEpochRecord{
		From:       admin,
		TxID:       tag.UID_FromTime(base),
		Member:     bob,
		SignPub:    "bobsign",
		EncryptPub: "bobenc0",
		Status:     MemberStatus_Active,
	}
	selfRotate := memberEpochRecord{
		From:       bob,
		TxID:       tag.UID_FromTime(base.Add(time.Minute)),
		Member:     bob,
		SignPub:    "bobsign",
		EncryptPub: "bobenc1",
		Status:     MemberStatus_Active,
	}
	restate := memberEpochRecord{
		From:       admin,
		TxID:       tag.UID_FromTime(base.Add(2 * time.Minute)),
		Member:     bob,
		SignPub:    "bobsign",
		EncryptPub: "bobenc0", // the rotation's stale re-wrap endpoint
		Status:     MemberStatus_Active,
	}

	orders := []struct {
		label   string
		records []memberEpochRecord
	}{
		{"in-order", []memberEpochRecord{seed, selfRotate, restate}},
		{"self-rotation delivered late", []memberEpochRecord{seed, restate, selfRotate}},
	}
	for _, order := range orders {
		binding := newMemberEpochBinding()
		for _, rec := range order.records {
			binding.OnNodeUpdate(memberEpochUpdate(t, rec))
		}
		if _, encryptPub, _ := mergedKeys(t, binding, bob); encryptPub != "bobenc1" {
			t.Errorf("%s: issuer restatement regressed the member's EncryptKey: %q want bobenc1", order.label, encryptPub)
		}
		// Idempotence: re-presenting every record (journal re-scan) changes nothing.
		for _, rec := range order.records {
			binding.OnNodeUpdate(memberEpochUpdate(t, rec))
		}
		if _, encryptPub, _ := mergedKeys(t, binding, bob); encryptPub != "bobenc1" {
			t.Errorf("%s: re-presentation regressed the merge: %q want bobenc1", order.label, encryptPub)
		}
	}

	// Control: the same in-order sequence on a plain-LWW binding answers the
	// restatement — the defect shape this merger exists to replace.  If this
	// ever fails, whole-value LWW changed underneath; re-evaluate the merger.
	plain := NewFoldBinding[*MemberEpoch](testMemberEpochAttr)
	plain.Bind(HeadNodeID)
	for _, rec := range []memberEpochRecord{seed, selfRotate, restate} {
		plain.OnNodeUpdate(memberEpochUpdate(t, rec))
	}
	if epoch, ok := plain.GetItem(bob); !ok || string(epoch.EncryptKey.PubKey) != "bobenc0" {
		t.Error("plain-LWW control no longer answers the restatement — baseline shifted")
	}
}

// TestMemberEpochMerge_StatusIssuerOwned: a member's self-record can rotate
// keys but never its own status — an issuer suspension survives a NEWER self
// "Active" record regardless of order.
func TestMemberEpochMerge_StatusIssuerOwned(t *testing.T) {
	admin, bob := tag.NewID(), tag.NewID()
	base := time.Now()
	binding := newMemberEpochBinding()
	binding.OnNodeUpdate(memberEpochUpdate(t, memberEpochRecord{
		From:       admin,
		TxID:       tag.UID_FromTime(base),
		Member:     bob,
		SignPub:    "bobsign",
		EncryptPub: "bobenc0",
		Status:     MemberStatus_Active,
	}))
	binding.OnNodeUpdate(memberEpochUpdate(t, memberEpochRecord{
		From:   admin,
		TxID:   tag.UID_FromTime(base.Add(time.Minute)),
		Member: bob,
		Status: MemberStatus_Suspended,
	}))
	binding.OnNodeUpdate(memberEpochUpdate(t, memberEpochRecord{
		From:       bob,
		TxID:       tag.UID_FromTime(base.Add(2 * time.Minute)),
		Member:     bob,
		SignPub:    "bobsign",
		EncryptPub: "bobenc1",
		Status:     MemberStatus_Active,
	}))
	signPub, encryptPub, gotStatus := mergedKeys(t, binding, bob)
	if gotStatus != MemberStatus_Suspended {
		t.Errorf("self record flipped issuer-owned status: %v want Suspended", gotStatus)
	}
	if signPub != "bobsign" || encryptPub != "bobenc1" {
		t.Errorf("suspension must not block the member's own key rotation: sign=%q encrypt=%q", signPub, encryptPub)
	}
}

// TestMemberEpochMerge_ServeLaneBaseOnly: an authorless record (op EditID !=
// carrying TxID — the snapshot serve shape) moves the record base but cannot
// touch custody-owned fields, in either direction.
func TestMemberEpochMerge_ServeLaneBaseOnly(t *testing.T) {
	admin, bob := tag.NewID(), tag.NewID()
	base := time.Now()
	binding := newMemberEpochBinding()
	binding.OnNodeUpdate(memberEpochUpdate(t, memberEpochRecord{
		From:       admin,
		TxID:       tag.UID_FromTime(base),
		Member:     bob,
		SignPub:    "bobsign",
		EncryptPub: "bobenc0",
		Status:     MemberStatus_Active,
	}))
	binding.OnNodeUpdate(memberEpochUpdate(t, memberEpochRecord{
		From:       bob,
		TxID:       tag.UID_FromTime(base.Add(time.Minute)),
		Member:     bob,
		SignPub:    "bobsign",
		EncryptPub: "bobenc1",
		Status:     MemberStatus_Active,
	}))
	// Serve-shaped restatement, newest EditID: base moves, custody holds.
	binding.OnNodeUpdate(memberEpochUpdate(t, memberEpochRecord{
		From:       admin,
		TxID:       tag.UID_FromTime(base.Add(2 * time.Minute)),
		ServeTxID:  tag.UID_FromTime(base.Add(time.Hour)),
		Member:     bob,
		SignPub:    "bobsign",
		EncryptPub: "bobenc0",
		Status:     MemberStatus_Active,
	}))
	if _, encryptPub, _ := mergedKeys(t, binding, bob); encryptPub != "bobenc1" {
		t.Errorf("authorless serve record moved a custody field: %q want bobenc1", encryptPub)
	}
}
