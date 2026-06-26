package amp

import (
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// TestResolveAccess_CyclicParentFailsClosed pins that a cyclic legislature parent chain
// TERMINATES (does not spin) and resolves to NotAllowed.  Without MaxACCParentDepth a self-
// or mutual-parent cycle would loop forever while a caller holds a read lock, wedging the
// whole access-control engine.  (If this regresses, the test hangs rather than failing.)
func TestResolveAccess_CyclicParentFailsClosed(t *testing.T) {
	member := tag.NewID()
	chA := tag.NewID()
	chB := tag.NewID()

	grant := func(level Access) *AccessGrants {
		return &AccessGrants{Grants: []*AccessGrant{{MemberTag: TagFromUID(member), Access: level}}}
	}

	// 2-cycle A↔B (each grants the member Admin).
	epochA := &ChannelEpoch{Channel: TagFromUID(chA), Parent: TagFromUID(chB), MemberGrants: grant(Access_Admin)}
	epochB := &ChannelEpoch{Channel: TagFromUID(chB), Parent: TagFromUID(chA), MemberGrants: grant(Access_Admin)}
	lookup := func(id tag.UID) *ChannelEpoch {
		switch id {
		case chA:
			return epochA
		case chB:
			return epochB
		}
		return nil
	}
	if got := ResolveAccess(member, epochA, lookup); got != Access_NotAllowed {
		t.Errorf("cyclic (A↔B) parent chain must fail closed (NotAllowed), got %v", got)
	}

	// 1-cycle A→A.
	epochSelf := &ChannelEpoch{Channel: TagFromUID(chA), Parent: TagFromUID(chA), MemberGrants: grant(Access_Admin)}
	selfLookup := func(id tag.UID) *ChannelEpoch {
		if id == chA {
			return epochSelf
		}
		return nil
	}
	if got := ResolveAccess(member, epochSelf, selfLookup); got != Access_NotAllowed {
		t.Errorf("self-parent (A→A) chain must fail closed (NotAllowed), got %v", got)
	}
}
