package std

// std.planetepoch_test.go pins PlanetEpochFromTx, the one PlanetEpoch
// extraction every reader shares: one op, at the planet head, or a refusal.

import (
	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

func TestPlanetEpochFromTx(t *testing.T) {
	epochAttr := Attr.LawPlanetEpoch.ID
	epochWith := func(label string) *amp.PlanetEpoch {
		return amp.EpochFromTerms(&amp.EpochTerms{
			EpochTag: amp.TagFromUID(tag.NewID()),
			Label:    label,
		})
	}
	txWith := func(nodes ...tag.UID) *amp.TxMsg {
		tx := amp.TxNew()
		for i, node := range nodes {
			label := string(rune('a' + i))
			err := tx.Upsert(node, epochAttr, tag.NewID(), epochWith(label))
			if err != nil {
				t.Fatal(err)
			}
		}
		return tx
	}

	if epoch, err := PlanetEpochFromTx(amp.TxNew()); epoch != nil || err != nil {
		t.Errorf("no PlanetEpoch op: got (%v, %v), want (nil, nil)", epoch, err)
	}
	epoch, err := PlanetEpochFromTx(txWith(amp.HeadNodeID))
	if err != nil || epoch == nil {
		t.Fatalf("one op at the head: got (%v, %v)", epoch, err)
	}
	if terms, _ := epoch.ParsedTerms(); terms.GetLabel() != "a" {
		t.Errorf("extracted Terms label %q, want %q", terms.GetLabel(), "a")
	}
	for _, refused := range []struct {
		name string
		tx   *amp.TxMsg
	}{
		{"two ops at the head", txWith(amp.HeadNodeID, amp.HeadNodeID)},
		{"one op away from the head", txWith(tag.NewID())},
	} {
		epoch, err := PlanetEpochFromTx(refused.tx)
		if epoch != nil || status.GetCode(err) != status.Code_BadRequest {
			t.Errorf("%s: got (%v, %v), want a BadRequest refusal",
				refused.name, epoch, err)
		}
	}
}
