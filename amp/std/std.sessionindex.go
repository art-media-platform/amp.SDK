package std

import (
	"sync"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
	"google.golang.org/protobuf/proto"
)

// SessionIndex mirrors one attr of a member's home-cabinet index (e.g.
// PlanetBinding) to its client's session channel: one meta op per item at
// (HeadNodeID, Attr, itemID).  Items Put before Open are held; Open, run once
// login is granted, sends them, and later Puts and Forgets push at once.
// Nothing pushes before the grant: a client takes the first session tx after
// its Login as the login reply.  A push fails only on a closing session; the
// failure is logged, never returned.
type SessionIndex struct {
	Attr tag.UID

	mu   sync.Mutex
	sess amp.Session // set by Open; nil while items are held
	held map[tag.UID]proto.Message
}

// Put records itemID's value: pushed now once Open has run, else held for it.
func (idx *SessionIndex) Put(itemID tag.UID, value proto.Message) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.sess != nil {
		idx.push(itemID, value)
		return
	}
	if idx.held == nil {
		idx.held = make(map[tag.UID]proto.Message)
	}
	idx.held[itemID] = value
}

// Forget removes itemID: a delete pushed now once Open has run, else the held
// item is dropped.
func (idx *SessionIndex) Forget(itemID tag.UID) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.sess != nil {
		idx.push(itemID, nil)
		return
	}
	delete(idx.held, itemID)
}

// Open sends every held item to sess and makes later Puts and Forgets push
// at once.  A second Open sends nothing.
func (idx *SessionIndex) Open(sess amp.Session) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.sess != nil {
		return
	}
	idx.sess = sess
	for itemID, value := range idx.held {
		idx.push(itemID, value)
	}
	idx.held = nil
}

// push sends one item; the caller holds mu, so pushes after Open keep call
// order, and the held set Open sends goes in no particular order.
func (idx *SessionIndex) push(itemID tag.UID, value proto.Message) {
	if err := PushSessionItem(idx.sess, idx.Attr, itemID, value); err != nil {
		idx.sess.Log().Warnf("session index %s: item %s not pushed: %v", idx.Attr.AsLabel(), itemID.AsLabel(), err)
	}
}
