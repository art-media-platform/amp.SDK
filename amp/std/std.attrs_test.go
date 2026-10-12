package std_test

import (
	"reflect"
	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/amp/std"
	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

// declaredAttrs lists every name std.Attr declares.
func declaredAttrs(t *testing.T) []tag.Name {
	t.Helper()
	names := []tag.Name(nil)
	attrs := reflect.ValueOf(std.Attr)
	for i := 0; i < attrs.NumField(); i++ {
		name, ok := attrs.Field(i).Interface().(tag.Name)
		if !ok {
			t.Fatalf("std.Attr.%s is not a tag.Name", attrs.Type().Field(i).Name)
		}
		names = append(names, name)
	}
	return names
}

// The exempt set is closed (AOM SD-security-sync.md §7.7): exactly these
// declared attrs stay planet-public on a Confidential planet.
func TestIsPlanetPublicAttr_ClosedSet(t *testing.T) {
	want := map[tag.UID]string{}
	for _, name := range []tag.Name{
		std.Attr.LawPlanetEpoch,
		std.Attr.LawMemberEpoch,
		std.Attr.LawChannelEpoch,
		std.Attr.LawEpochLink,
		std.Attr.LawPlanetOrigin,
		std.Attr.LawEquivalence,
		std.Attr.Brand,
		std.Attr.PlanetInvitePolicy,
		std.Attr.PlanetInviteRedemption,
		std.Attr.NameServiceRecord,
		std.Attr.FederationDirectory,
		std.Attr.PlanetStorageOpts,
		std.Attr.NodeLogEntry,
		std.Attr.NodeMessage,
		std.Attr.LedgerAttestation,
	} {
		want[name.ID] = name.Text
	}
	got := map[tag.UID]string{}
	for _, name := range declaredAttrs(t) {
		if std.IsPlanetPublicAttr(name.ID) {
			got[name.ID] = name.Text
		}
	}
	for attrID, text := range want {
		if _, ok := got[attrID]; !ok {
			t.Errorf("%s is not planet-public", text)
		}
	}
	for attrID, text := range got {
		if _, ok := want[attrID]; !ok {
			t.Errorf("%s is planet-public outside the closed set", text)
		}
	}
	for _, sealed := range []tag.Name{
		std.Attr.LawWithdraw,
		std.Attr.BlobRef,
		std.Attr.NodeBlobs,
		std.Attr.NodeCredentials,
		std.Attr.PlanetBinding,
		std.Attr.FollowBinding,
		std.Attr.ItemLabel,
	} {
		if std.IsPlanetPublicAttr(sealed.ID) {
			t.Errorf("%s is content and must not be planet-public", sealed.Text)
		}
	}
}

// Every governance-law attr is planet-public: a keyless node reads the
// governance spine.
func TestIsPlanetPublicAttr_CoversGovernanceLaw(t *testing.T) {
	for _, name := range declaredAttrs(t) {
		if std.IsGovernanceLawAttr(name.ID) && !std.IsPlanetPublicAttr(name.ID) {
			t.Errorf("governance-law attr %s is not planet-public", name.Text)
		}
	}
}

// The node log rail's declared attrs mint the UIDs its writers addressed
// before the declaration: an .sdl attr UID is tag.UID_FromName of its text.
func TestNodeLogAttrsMintByName(t *testing.T) {
	for _, name := range []tag.Name{
		std.Attr.NodeLog,
		std.Attr.NodeLogEntry,
		std.Attr.NodeMessages,
		std.Attr.NodeMessage,
	} {
		if name.ID != tag.UID_FromName(name.Text) {
			t.Errorf("%s mints %s, want %s", name.Text, name.ID.Base32(), tag.UID_FromName(name.Text).Base32())
		}
	}
}

func TestTxSealDomain(t *testing.T) {
	cases := []struct {
		label string
		attrs []tag.UID
		want  std.SealDomain
	}{
		{
			label: "empty",
			want:  std.SealDomain_Empty,
		},
		{
			label: "planet-public",
			attrs: []tag.UID{std.Attr.LawMemberEpoch.ID, std.Attr.Brand.ID},
			want:  std.SealDomain_PlanetPublic,
		},
		{
			label: "content",
			attrs: []tag.UID{std.Attr.ItemLabel.ID, std.Attr.BlobRef.ID},
			want:  std.SealDomain_Content,
		},
		{
			label: "mixed",
			attrs: []tag.UID{std.Attr.LawChannelEpoch.ID, std.Attr.ItemLabel.ID},
			want:  std.SealDomain_Mixed,
		},
	}
	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			tx := amp.TxNew()
			tx.SetTxID(tag.NowID())
			for _, attrID := range tc.attrs {
				if err := tx.Upsert(amp.HeadNodeID, attrID, tag.NewID(), amp.TagText("text/plain", tc.label)); err != nil {
					t.Fatal(err)
				}
			}
			if got := std.TxSealDomain(tx); got != tc.want {
				t.Fatalf("TxSealDomain = %v, want %v", got, tc.want)
			}
		})
	}
}
