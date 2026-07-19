package std

import (
	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// amp.Terminal channel defs (AOM AD-app-terminal.md).  The TermIO tape rides a
// Tape attr: the journal is the store, EditID is the time axis, and the
// ItemID is the track lane.  Producer and every reader import these — the
// registration is the shared-const site.
var (
	// TerminalTape carries a session's TermIO frames (EditType_Tape): zero
	// cabinet rows; serve replays the journal window (SD-planet-storage §8.1).
	TerminalTape = RegisterAttrTape(Attr.ItemSeries, &amp.TermIO{}, "main.terminal")

	// TerminalHead is the folded live-keyframe cell (Folded, K=1) — the
	// instant re-attach anchor a fresh console loads before tape replay.
	TerminalHead = RegisterAttr(Attr.ItemSeries, &amp.TermIO{}, "main.terminal.head")
)

// Terminal track lanes — constant ItemIDs on the tape attr (t rides EditID,
// freeing the ItemID to discriminate lanes; AD-app-terminal §4.2).
var (
	TerminalTrackOut      = tag.UID_HashLiteral([]byte("amp.terminal/track/out"))
	TerminalTrackIn       = tag.UID_HashLiteral([]byte("amp.terminal/track/in"))
	TerminalTrackKeyframe = tag.UID_HashLiteral([]byte("amp.terminal/track/keyframe"))
)
