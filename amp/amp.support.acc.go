package amp

import (
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// ResolveAccess determines the effective Access level for a member on a channel.
// Walks the legislature chain from the channel's ChannelEpoch up through each
// Parent, intersecting permissions at every level (most restrictive wins).
//
// lookupEpoch retrieves the ChannelEpoch for a given channel UID.  If it returns
// nil anywhere in the chain, the chain is broken and NotAllowed is returned —
// a missing ancestor must never fail-open.
func ResolveAccess(memberID tag.UID, channelEpoch *ChannelEpoch, lookupEpoch func(channelID tag.UID) *ChannelEpoch) Access {
	if channelEpoch == nil {
		return Access_NotAllowed
	}

	level := resolveMemberGrants(memberID, channelEpoch)

	parent := channelEpoch.Parent
	for parent != nil {
		parentID := parent.UID()
		if parentID.IsNil() {
			break
		}
		parentEpoch := lookupEpoch(parentID)
		if parentEpoch == nil {
			return Access_NotAllowed
		}
		parentLevel := resolveMemberGrants(memberID, parentEpoch)
		level = minAccess(level, parentLevel)
		parent = parentEpoch.Parent
	}

	return level
}

// HasAccess checks if a member meets at least the required access level.
func HasAccess(memberID tag.UID, required Access, channelEpoch *ChannelEpoch, lookupEpoch func(channelID tag.UID) *ChannelEpoch) bool {
	return ResolveAccess(memberID, channelEpoch, lookupEpoch) >= required
}

// resolveMemberGrants looks up a member's access in a single ChannelEpoch.
// Explicit MemberGrants win over DefaultGrants.
func resolveMemberGrants(memberID tag.UID, epoch *ChannelEpoch) Access {
	if epoch.MemberGrants != nil {
		for _, grant := range epoch.MemberGrants.Grants {
			if grant.MemberTag != nil && grant.MemberTag.UID() == memberID {
				return grant.Access
			}
		}
	}
	if epoch.DefaultGrants != nil {
		for _, grant := range epoch.DefaultGrants.Grants {
			return grant.Access
		}
	}
	return Access_NotAllowed
}

func minAccess(level1, level2 Access) Access {
	if level1 < level2 {
		return level1
	}
	return level2
}
