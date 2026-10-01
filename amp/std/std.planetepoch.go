package std

import (
	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/status"
)

// PlanetEpochFromTx returns tx's PlanetEpoch — its one LawPlanetEpoch op,
// which must sit at amp.HeadNodeID — or nil when tx carries none.  A second
// PlanetEpoch op, or one away from the planet head, refuses the tx, and an op
// that does not unmarshal returns its error: every reader of the epoch takes
// the same op (SD-channel-governance §3.3).
func PlanetEpochFromTx(tx *amp.TxMsg) (*amp.PlanetEpoch, error) {
	epoch := (*amp.PlanetEpoch)(nil)
	for opIndex, op := range tx.Ops {
		if op.Addr.AttrID != Attr.LawPlanetEpoch.ID {
			continue
		}
		if epoch != nil {
			return nil, status.Code_BadRequest.Error(
				"std: tx carries more than one PlanetEpoch op")
		}
		if op.Addr.NodeID != amp.HeadNodeID {
			return nil, status.Code_BadRequest.Error(
				"std: PlanetEpoch op away from the planet head")
		}
		epoch = &amp.PlanetEpoch{}
		if err := tx.UnmarshalOpValue(opIndex, epoch); err != nil {
			return nil, err
		}
	}
	return epoch, nil
}
