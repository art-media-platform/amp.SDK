package std

// std.blobprogress.go — the H4 band read of a BlobProgress state value (AD-app-www §5.6):
// a client renders a value it has no name for by its band alone, so a newer host's state
// never reads as unset.  The band edges are the generated BlobProgressBand* consts; this
// is their one interpretation site.

// BlobStateBand is the band a BlobPullState, BlobReaderState or BlobPullReason value falls in.
type BlobStateBand int32

const (
	BlobStateBand_Unset    BlobStateBand = iota // 0, or a value past BandMax
	BlobStateBand_Waiting                       // 1–15: the pull moves or is about to
	BlobStateBand_Held                          // 16–23: blocked on something that may clear
	BlobStateBand_Terminal                      // 24–31: the pull ended
)

// BlobStateBandOf reads a state value's band.
func BlobStateBandOf(value int32) BlobStateBand {
	switch {
	case value >= BlobProgressBandTerminalMin && value <= BlobProgressBandMax:
		return BlobStateBand_Terminal
	case value >= BlobProgressBandHeldMin && value < BlobProgressBandTerminalMin:
		return BlobStateBand_Held
	case value >= BlobProgressBandWaitingMin && value < BlobProgressBandHeldMin:
		return BlobStateBand_Waiting
	}
	return BlobStateBand_Unset
}

func (band BlobStateBand) String() string {
	switch band {
	case BlobStateBand_Waiting:
		return "waiting"
	case BlobStateBand_Held:
		return "held"
	case BlobStateBand_Terminal:
		return "terminal"
	}
	return "unset"
}
