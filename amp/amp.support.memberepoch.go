package amp

// MemberEpoch item merge — per-field custody for the (HeadNode, LawMemberEpoch,
// memberID) governance item, replacing whole-value LWW wherever a binding
// observes MemberEpoch records (SD-channel-governance §3.2: member keys are
// member-owned — an issuer seeds a key on admission, only the member's own
// record rotates it thereafter; Status is issuer-owned).  The clock semantics
// mirror the vault member-key cache (amp.planet indexMemberEpochsFromTx): each
// custody field advances monotonically by its OWN EditID, so an issuer
// restatement at a newer EditID can never regress a member's self-rotated key,
// and a member's in-flight self-rotation delivered late still wins its field.
//
// Two lanes, judged per record:
//
//   - Custody lane — the op's EditID equals its carrying TxID: a live authored
//     governance record (MemberEpoch ops are authored with a nil edit seed, so
//     EditID == TxID exactly; SD-security-sync §4.4), whose FromID names the
//     author.  Custody rules apply.
//   - Base lane — everything else: a snapshot serve re-emission rides a
//     synthetic tx (its own TxID, the stamped original EditID on the op), and a
//     forged edit seed fails the same test.  The newest record moves the merged
//     record's base — the whole-value LWW this merger otherwise replaces — but
//     never a custody field an earlier custody-lane record already owns: an
//     authorless record cannot be custody-judged.
//
// C# twin: the Unity binding layer keeps whole-value LWW until the seated pass
// ports this merger; named residual, not silently divergent — the Go side is
// the only binding consumer of MemberEpoch custody today.

import (
	"google.golang.org/protobuf/proto"

	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// memberEpochState is one item's fold state: the newest record (the base) plus
// the custody-owned fields, each under its own high-water clock.
type memberEpochState struct {
	baseEdit    tag.UID
	base        *MemberEpoch
	signEdit    tag.UID
	signKey     *safe.KeyRef
	encryptEdit tag.UID
	encryptKey  *safe.KeyRef
	statusEdit  tag.UID
	status      MemberStatus
	epoch       *Tag
}

// MemberEpochMerger implements ItemMerger[*MemberEpoch] (one instance per
// binding — the clock state is per-item and not shared across bindings).
type MemberEpochMerger struct {
	state map[tag.UID]*memberEpochState
}

func NewMemberEpochMerger() *MemberEpochMerger {
	return &MemberEpochMerger{
		state: make(map[tag.UID]*memberEpochState, 8),
	}
}

func (merger *MemberEpochMerger) DropItem(itemID tag.UID) {
	delete(merger.state, itemID)
}

func declaredKey(key *safe.KeyRef) bool { return key != nil && len(key.PubKey) > 0 }

// MergeItem folds one arriving MemberEpoch record; see the file comment for
// the lane and custody rules.
func (merger *MemberEpochMerger) MergeItem(arrival AttrItem[*MemberEpoch], prev *MemberEpoch, hasPrev bool) (*MemberEpoch, bool) {
	incoming := arrival.Value
	memberID := arrival.Addr.ItemID
	editID := arrival.Addr.EditID

	custodyLane := arrival.Tx != nil && arrival.Tx.TxID() == editID
	isSelf := custodyLane && arrival.Tx.FromID() == memberID

	state := merger.state[memberID]
	if state == nil {
		state = &memberEpochState{}
		merger.state[memberID] = state
	}
	changed := false

	// Base lane: the newest record carries every non-custody field.
	if state.base == nil || state.baseEdit.CompareTo(editID) < 0 {
		state.base = proto.Clone(incoming).(*MemberEpoch)
		state.baseEdit = editID
		changed = true
	}

	// Custody lane, mirroring the vault member-key cache clause for clause.
	// EncryptKey custody sits inside the SigningKey-declared gate exactly as
	// the vault indexes it (the D-encryptkey-gated posture rides along).
	if custodyLane {
		if declaredKey(incoming.SigningKey) {
			priorSignEmpty := !declaredKey(state.signKey)
			if (isSelf || priorSignEmpty) && state.signEdit.CompareTo(editID) < 0 {
				state.signKey = proto.Clone(incoming.SigningKey).(*safe.KeyRef)
				state.signEdit = editID
				changed = true
			}
			if declaredKey(incoming.EncryptKey) {
				priorEncryptEmpty := !declaredKey(state.encryptKey)
				if (isSelf || priorEncryptEmpty) && state.encryptEdit.CompareTo(editID) < 0 {
					state.encryptKey = proto.Clone(incoming.EncryptKey).(*safe.KeyRef)
					state.encryptEdit = editID
					changed = true
				}
			}
		}
		// Status (and the Epoch tag riding it) is issuer-owned; the first
		// record for a member bootstraps it.
		if (!isSelf || !hasPrev) && state.statusEdit.CompareTo(editID) < 0 {
			state.status = incoming.Status
			state.statusEdit = editID
			if incoming.Epoch != nil {
				state.epoch = proto.Clone(incoming.Epoch).(*Tag)
			}
			changed = true
		}
	}

	if !changed {
		return prev, false
	}
	return state.materialize(), true
}

// materialize projects the fold state into one served record: the base record
// with every custody-owned field overriding it.
func (state *memberEpochState) materialize() *MemberEpoch {
	out := proto.Clone(state.base).(*MemberEpoch)
	if state.signEdit.IsSet() {
		out.SigningKey = proto.Clone(state.signKey).(*safe.KeyRef)
	}
	if state.encryptEdit.IsSet() {
		out.EncryptKey = proto.Clone(state.encryptKey).(*safe.KeyRef)
	}
	if state.statusEdit.IsSet() {
		out.Status = state.status
		if state.epoch != nil {
			out.Epoch = proto.Clone(state.epoch).(*Tag)
		}
	}
	return out
}
