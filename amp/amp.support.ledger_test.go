package amp_test

import (
	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
	_ "github.com/art-media-platform/amp.SDK/stdlib/safe/poly25519" // HashKit registration
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// TestAttestationKey_Convergence verifies that two observers computing the
// same AttestationKey produce the same ItemID — this is the load-bearing
// property for CRDT convergence on the planet ledger.
func TestAttestationKey_Convergence(t *testing.T) {
	subject := tag.NewID()
	citedTx := tag.NewID()

	key1 := amp.AttestationKey{
		Type:         amp.AttestationType_Strike,
		Subject:      subject,
		CitedTxID:    citedTx,
		CitedOpIndex: 2,
	}
	key2 := amp.AttestationKey{
		Type:         amp.AttestationType_Strike,
		Subject:      subject,
		CitedTxID:    citedTx,
		CitedOpIndex: 2,
	}
	if key1.ItemID() != key2.ItemID() {
		t.Fatalf("convergence broken: %v != %v", key1.ItemID(), key2.ItemID())
	}
}

// TestAttestationKey_Divergence verifies that any difference in the cited
// facts produces a different ItemID — otherwise false-positive merging would
// let two distinct violations collapse into one record.
func TestAttestationKey_Divergence(t *testing.T) {
	base := amp.AttestationKey{
		Type:         amp.AttestationType_Strike,
		Subject:      tag.UID{1, 1},
		CitedTxID:    tag.UID{2, 2},
		CitedOpIndex: 0,
	}

	cases := []struct {
		name string
		key  amp.AttestationKey
	}{
		{"different type", amp.AttestationKey{
			Type: amp.AttestationType_Endorsement, Subject: base.Subject, CitedTxID: base.CitedTxID, CitedOpIndex: base.CitedOpIndex,
		}},
		{"different subject", amp.AttestationKey{
			Type: base.Type, Subject: tag.UID{9, 9}, CitedTxID: base.CitedTxID, CitedOpIndex: base.CitedOpIndex,
		}},
		{"different tx", amp.AttestationKey{
			Type: base.Type, Subject: base.Subject, CitedTxID: tag.UID{8, 8}, CitedOpIndex: base.CitedOpIndex,
		}},
		{"different op index", amp.AttestationKey{
			Type: base.Type, Subject: base.Subject, CitedTxID: base.CitedTxID, CitedOpIndex: 7,
		}},
	}

	baseID := base.ItemID()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.key.ItemID() == baseID {
				t.Fatalf("ItemID collision: %s produced same ID as base", tc.name)
			}
		})
	}
}
