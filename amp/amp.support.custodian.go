package amp

import (
	"strings"

	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// Custodial (watchdog-class) grant support — the shared judgment every
// enforcement site consumes: the ACC CustodianRule, the advisory session
// check, and a watchdog's own grant self-check (SD-channel-governance §9).

// GrantIsCustodial reports whether grant carries the watchdog-class capability
// record or data-plane scope whitelist.  A grant carrying either fences its
// member on the data plane; the Access rung is ordering only and is never the
// test (SD-channel-governance §9.1).
func GrantIsCustodial(grant *AccessGrant) bool {
	return grant != nil && (len(grant.Capabilities) > 0 || len(grant.Scopes) > 0)
}

// GrantHasCapability reports whether grant's capability record names verb.
func GrantHasCapability(grant *AccessGrant, verb WatchdogCapability) bool {
	if grant == nil {
		return false
	}
	for _, held := range grant.Capabilities {
		if held == verb {
			return true
		}
	}
	return false
}

// ScopeAdmits reports whether one of scopes admits a write at (nodeID,
// attrID).  A row admits when nodeID equals the fold of its NodeSpace
// word-chain AND attrID resolves — through reg — to a registered attr whose
// canonic name equals or word-extends the row's AttrSpace prefix
// (SD-channel-governance §9.2).
//
// An AttrID on the wire is a fold UID and carries no subtree relation, so the
// prefix test runs on the resolved canonic STRING; anything the registry
// cannot resolve — and any empty or unresolvable row half — admits nothing:
// the whitelist fails closed.
func ScopeAdmits(scopes []*AttrScope, reg Registry, nodeID, attrID tag.UID) bool {
	if reg == nil || nodeID.IsNil() || attrID.IsNil() {
		return false
	}
	def, ok := reg.FindAttr(attrID)
	if !ok {
		return false // unregistered attr — no canonic name to judge; out of scope
	}
	attrCanonic := def.Canonic()
	for _, row := range scopes {
		if row == nil || row.NodeSpace == "" || row.AttrSpace == "" {
			continue // a row half the grant does not name matches nothing
		}
		if (tag.Name{}).With(row.NodeSpace).ID != nodeID {
			continue
		}
		prefix := (tag.Name{}).With(row.AttrSpace).Canonic()
		if prefix == "" {
			continue
		}
		if attrCanonic == prefix || strings.HasPrefix(attrCanonic, prefix+tag.CanonicSeparator) {
			return true
		}
	}
	return false
}
