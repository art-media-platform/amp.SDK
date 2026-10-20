package amp_test

// epoch_next_terms_test.go pins the successor-Terms derivation
// (amp.NextEpochTerms, AssembleSuccessor, VerifySuccessor;
// SD-security-sync §4.4, ZO-design-conventions §2.8).  Every expected value
// is a literal written here, or a canonical fixture with the expected changes
// applied by hand — never a value the function under test minted.

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/art-media-platform/amp.SDK/amp"
	"github.com/art-media-platform/amp.SDK/stdlib/safe"
	"github.com/art-media-platform/amp.SDK/stdlib/status"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// successorTag stands in for the successor's NowID-minted EpochTag.
func successorTag() *amp.Tag {
	return tagWithUID(0x5E, 0x04)
}

// withUnknown appends field num, length-delimited and carrying val, to msg's
// unknown fields: a field this binary does not know.
func withUnknown(msg proto.Message, num protowire.Number, val string) {
	raw := msg.ProtoReflect().GetUnknown()
	raw = protowire.AppendTag(raw, num, protowire.BytesType)
	raw = protowire.AppendString(raw, val)
	msg.ProtoReflect().SetUnknown(raw)
}

// unknownNumbers lists msg's unknown field numbers in wire order.
func unknownNumbers(t *testing.T, msg proto.Message) []protowire.Number {
	t.Helper()
	raw := msg.ProtoReflect().GetUnknown()
	nums := []protowire.Number{}
	for len(raw) > 0 {
		num, _, size := protowire.ConsumeField(raw)
		if size < 0 {
			t.Fatalf("malformed unknown field: %v", protowire.ParseError(size))
		}
		nums = append(nums, num)
		raw = raw[size:]
	}
	return nums
}

// rotationPrev is a predecessor's Terms at height 3 carrying a governed term
// of each carry shape: a string, a message, an enum Paused, an int 2, a
// VaultConfig with a tunable, LicenseClass (33) and a marker (34).
func rotationPrev() *amp.EpochTerms {
	return &amp.EpochTerms{
		TermsSchema:   1,
		EpochTag:      tagWithUID(0x5E, 0x03),
		PreviousEpoch: tagWithUID(0x5E, 0x02),
		EpochHeight:   3,
		CryptoKitID_0: safe.Crypto.Poly25519.ID[0],
		CryptoKitID_1: safe.Crypto.Poly25519.ID[1],
		Label:         "Prior",
		Mark: &amp.BrandMark{
			Identity: &amp.BrandIdentity{
				AppName:    "Old",
				URLSchemes: []string{"old"},
			},
		},
		Seal:               amp.SealState_Paused,
		RequiredSignatures: 2,
		VaultConfig: &amp.VaultConfig{
			MaxTxMsgSize: 7,
			VaultAddrs: []*amp.VaultAddr{{
				Transport: "tcp",
				Address:   []byte("a:1"),
			}},
		},
		LicenseClass:       tagWithUID(0x33, 0x33),
		ContentSealedSince: tagWithUID(0x5E, 0x01),
	}
}

// rotationNext is rotationPrev's carry-only successor, written by hand: the
// chain advanced, every other term equal.
func rotationNext() *amp.EpochTerms {
	return &amp.EpochTerms{
		TermsSchema:   1,
		EpochTag:      successorTag(),
		PreviousEpoch: tagWithUID(0x5E, 0x03),
		EpochHeight:   4,
		CryptoKitID_0: safe.Crypto.Poly25519.ID[0],
		CryptoKitID_1: safe.Crypto.Poly25519.ID[1],
		Label:         "Prior",
		Mark: &amp.BrandMark{
			Identity: &amp.BrandIdentity{
				AppName:    "Old",
				URLSchemes: []string{"old"},
			},
		},
		Seal:               amp.SealState_Paused,
		RequiredSignatures: 2,
		VaultConfig: &amp.VaultConfig{
			MaxTxMsgSize: 7,
			VaultAddrs: []*amp.VaultAddr{{
				Transport: "tcp",
				Address:   []byte("a:1"),
			}},
		},
		LicenseClass:       tagWithUID(0x33, 0x33),
		ContentSealedSince: tagWithUID(0x5E, 0x01),
	}
}

// nextTerms runs NextEpochTerms over prev with change, the successor tag fed
// in, failing the test on a refusal.
func nextTerms(t *testing.T, prev *amp.EpochTerms, change amp.TermsChange) *amp.EpochTerms {
	t.Helper()
	change.EpochTag = successorTag()
	next, err := amp.NextEpochTerms(prev, change)
	if err != nil {
		t.Fatalf("NextEpochTerms: %v", err)
	}
	return next
}

// assertTerms fails unless got is proto.Equal to want.
func assertTerms(t *testing.T, label string, got, want *amp.EpochTerms) {
	t.Helper()
	if !proto.Equal(got, want) {
		t.Errorf("%s:\n got: %v\nwant: %v", label, got, want)
	}
}

// LicenseClass (33) and an unknown carry-band field (40) carry.
func TestNextEpochTerms_CarriesLicenseClassAndUnknown(t *testing.T) {
	prev := rotationPrev()
	withUnknown(prev, 40, "carry-40")
	want := rotationNext()
	withUnknown(want, 40, "carry-40")
	assertTerms(t, "carry-only successor", nextTerms(t, prev, amp.TermsChange{}), want)
}

// An unknown epoch-local field (540) drops; an unknown
// must-understand (1540) or unallocated (2040) field refuses, naming it.
func TestNextEpochTerms_UnknownFieldBands(t *testing.T) {
	prev := rotationPrev()
	withUnknown(prev, 540, "local-540")
	assertTerms(t, "540 dropped", nextTerms(t, prev, amp.TermsChange{}), rotationNext())

	for _, num := range []protowire.Number{1540, 2040} {
		prev := rotationPrev()
		withUnknown(prev, num, "must")
		_, err := amp.NextEpochTerms(prev, amp.TermsChange{
			EpochTag: successorTag(),
		})
		if !errors.Is(err, amp.ErrTermsMustUnderstand) || !strings.Contains(err.Error(), fmt.Sprint(num)) {
			t.Errorf("unknown %d: err = %v, want ErrTermsMustUnderstand naming %d", num, err, num)
		}
	}
}

// A template carrying a field this binary does not know, at
// any depth, refuses, naming the number.
func TestNextEpochTerms_TemplateUnknownRefused(t *testing.T) {
	top := &amp.EpochTerms{
		Label: "New",
	}
	withUnknown(top, 40, "set-by-a-newer-client")
	nested := &amp.EpochTerms{
		VaultConfig: &amp.VaultConfig{},
	}
	withUnknown(nested.VaultConfig, 99, "nested")
	cases := []struct {
		name   string
		change amp.TermsChange
		want   string
	}{
		{"top level", amp.TermsChange{Template: top}, "40"},
		{
			name: "top level, paths",
			change: amp.TermsChange{
				Template: top,
				Paths:    []string{"Label"},
			},
			want: "40",
		},
		{"nested", amp.TermsChange{Template: nested}, "VaultConfig.field(s) [99]"},
	}
	for _, c := range cases {
		c.change.EpochTag = successorTag()
		_, err := amp.NextEpochTerms(rotationPrev(), c.change)
		if !errors.Is(err, amp.ErrTermsTemplate) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want ErrTermsTemplate naming %q", c.name, err, c.want)
		}
	}
}

// A path with no template value clears, so Seal resumes Open
// and RequiredSignatures returns to 0; without paths a zero term carries.
func TestNextEpochTerms_PathClears(t *testing.T) {
	want := rotationNext()
	want.Seal = amp.SealState_Open
	want.RequiredSignatures = 0
	got := nextTerms(t, rotationPrev(), amp.TermsChange{
		Paths: []string{"Seal", "RequiredSignatures"},
	})
	assertTerms(t, "?set=Seal&set=RequiredSignatures", got, want)

	got = nextTerms(t, rotationPrev(), amp.TermsChange{
		Template: &amp.EpochTerms{
			Seal: amp.SealState_Open,
		},
	})
	assertTerms(t, "no paths: zero terms carry", got, rotationNext())
}

// A nested path replaces its leaf and keeps the leaf's siblings and the
// parent's unknown fields; a leaf list or message replaces whole, never
// merged.  Without paths a set message term replaces whole.
func TestNextEpochTerms_PathLeafReplacesWhole(t *testing.T) {
	prev := rotationPrev()
	withUnknown(prev.VaultConfig, 99, "nested-future")
	template := &amp.EpochTerms{
		VaultConfig: &amp.VaultConfig{
			VaultAddrs: []*amp.VaultAddr{{
				Transport: "tcp",
				Address:   []byte("b:2"),
			}},
		},
		Mark: &amp.BrandMark{
			Identity: &amp.BrandIdentity{
				AppName: "New",
			},
		},
	}

	want := rotationNext()
	want.VaultConfig = &amp.VaultConfig{
		MaxTxMsgSize: 7,
		VaultAddrs: []*amp.VaultAddr{{
			Transport: "tcp",
			Address:   []byte("b:2"),
		}},
	}
	withUnknown(want.VaultConfig, 99, "nested-future")
	want.Mark = &amp.BrandMark{
		Identity: &amp.BrandIdentity{
			AppName: "New",
		},
	}
	got := nextTerms(t, prev, amp.TermsChange{
		Template: template,
		Paths:    []string{"VaultConfig.VaultAddrs", "Mark.Identity"},
	})
	assertTerms(t, "nested paths", got, want)

	wholeWant := rotationNext()
	wholeWant.VaultConfig = &amp.VaultConfig{
		VaultAddrs: []*amp.VaultAddr{{
			Transport: "tcp",
			Address:   []byte("b:2"),
		}},
	}
	wholeWant.Mark = &amp.BrandMark{
		Identity: &amp.BrandIdentity{
			AppName: "New",
		},
	}
	got = nextTerms(t, prev, amp.TermsChange{
		Template: template,
	})
	assertTerms(t, "no paths: whole-value replacement", got, wholeWant)
}

// The path grammar's refusals, each failing closed and naming what it
// refuses; both suite halves together, and CodexEdition named whole, are
// accepted.
func TestNextEpochTerms_PathRefusals(t *testing.T) {
	labelled := &amp.EpochTerms{
		Label: "New",
	}
	siblings := &amp.EpochTerms{
		VaultConfig: &amp.VaultConfig{
			MaxTxMsgSize: 9,
			VaultAddrs: []*amp.VaultAddr{{
				Transport: "tcp",
				Address:   []byte("b:2"),
			}},
		},
	}
	emptyMessage := &amp.EpochTerms{
		Label:       "New",
		VaultConfig: &amp.VaultConfig{},
	}
	cases := []struct {
		name     string
		template *amp.EpochTerms
		paths    []string
		want     string
	}{
		{"unknown segment", nil, []string{"LicenseTier"}, `"LicenseTier" is not a field this binary knows`},
		{"empty segment", nil, []string{"VaultConfig."}, `"" is not a field this binary knows`},
		{"through a repeated field", nil, []string{"VaultConfig.VaultAddrs.Transport"}, "VaultAddrs is not a singular message"},
		{"through a scalar", nil, []string{"Label.Text"}, "Label is not a singular message"},
		{"chain field", nil, []string{"EpochHeight"}, "EpochHeight (4) is not settable"},
		{"assembled field", nil, []string{"HashKit"}, "HashKit (5) is not settable"},
		{"one suite half", nil, []string{"CryptoKitID_0"}, "both CryptoKitID halves"},
		{"predecessor-only", nil, []string{"ContentSealedSince"}, "ContentSealedSince (34) is not settable"},
		{"carry-only LicenseClass", nil, []string{"LicenseClass"}, "LicenseClass (33) is not settable"},
		{"into a template-only term", nil, []string{"CodexEdition.PlanetID_0"}, "CodexEdition is set whole"},
		{"a template term no path names", labelled, []string{"Seal"}, "the template sets Label, which no path names"},
		{"a sibling leaf no path names", siblings, []string{"VaultConfig.VaultAddrs"}, "sets VaultConfig.MaxTxMsgSize, which no path names"},
		{"an empty message no path names", emptyMessage, []string{"Label"}, "the template sets VaultConfig, which no path names"},
	}
	for _, c := range cases {
		_, err := amp.NextEpochTerms(rotationPrev(), amp.TermsChange{
			Template: c.template,
			Paths:    c.paths,
			EpochTag: successorTag(),
		})
		if !errors.Is(err, amp.ErrTermsTemplate) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want ErrTermsTemplate naming %q", c.name, err, c.want)
		}
	}

	template := &amp.EpochTerms{}
	template.SetCryptoKitID(safe.Crypto.P256.ID)
	got := nextTerms(t, rotationPrev(), amp.TermsChange{
		Template: template,
		Paths:    []string{"CryptoKitID_0", "CryptoKitID_1"},
	})
	want := rotationNext()
	want.SetCryptoKitID(safe.Crypto.P256.ID)
	assertTerms(t, "both suite halves", got, want)

	edition := &amp.EpochTerms{
		CodexEdition: &amp.Address{
			NodeID_0: 7,
		},
	}
	got = nextTerms(t, rotationPrev(), amp.TermsChange{
		Template: edition,
		Paths:    []string{"CodexEdition"},
	})
	want = rotationNext()
	want.CodexEdition = &amp.Address{
		NodeID_0: 7,
	}
	assertTerms(t, "CodexEdition named whole", got, want)
}

// Bands classify top-level numbers only: an unknown epoch-local field nested
// inside a carried message travels with the message.
func TestNextEpochTerms_NestedUnknownCarries(t *testing.T) {
	prev := rotationPrev()
	withUnknown(prev.VaultConfig, 540, "nested-local")
	want := rotationNext()
	withUnknown(want.VaultConfig, 540, "nested-local")
	assertTerms(t, "nested 540", nextTerms(t, prev, amp.TermsChange{}), want)
}

// A predecessor whose unknown bytes do not parse refuses as a data failure;
// a new EpochTag equal to the predecessor's own refuses, here and in
// VerifySuccessor.
func TestNextEpochTerms_MalformedOrSelfLoopRefused(t *testing.T) {
	prev := rotationPrev()
	prev.ProtoReflect().SetUnknown(protowire.AppendTag(nil, 40, protowire.BytesType))
	_, err := amp.NextEpochTerms(prev, amp.TermsChange{
		EpochTag: successorTag(),
	})
	if !status.IsError(err, status.Code_DataFailure) {
		t.Errorf("malformed unknown bytes: err = %v, want DataFailure", err)
	}

	_, err = amp.NextEpochTerms(rotationPrev(), amp.TermsChange{
		EpochTag: rotationPrev().GetEpochTag(),
	})
	if !status.IsError(err, status.Code_BadRequest) || !strings.Contains(err.Error(), "predecessor's own") {
		t.Errorf("self-loop: err = %v, want a BadRequest naming the predecessor's own EpochTag", err)
	}

	genesis := genesisEpoch(t)
	prevTerms, err := genesis.ParsedTerms()
	if err != nil {
		t.Fatal(err)
	}
	loopTerms := proto.Clone(prevTerms).(*amp.EpochTerms)
	loopTerms.PreviousEpoch = prevTerms.EpochTag
	loopTerms.EpochHeight = 1
	loop, err := amp.AssembleSuccessor(genesis, loopTerms)
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.VerifySuccessor(genesis, amp.TermsChange{}); err == nil || !strings.Contains(err.Error(), "predecessor's own") {
		t.Errorf("VerifySuccessor of a self-loop: err = %v, want a refusal naming the predecessor's own EpochTag", err)
	}
}

// The no-path template classes: a carried term replaces when set, the suite
// pair moves as one, CodexEdition is the template's only, chain fields are
// ignored, and LicenseClass (until it gains a writer) and
// ContentSealedSince refuse.
func TestNextEpochTerms_TemplateClasses(t *testing.T) {
	edition := &amp.Address{
		PlanetID_0: 0xED,
	}
	template := &amp.EpochTerms{
		EpochHeight:  99,
		Label:        "Renamed",
		CodexEdition: edition,
	}
	template.SetCryptoKitID(safe.Crypto.P256.ID)
	want := rotationNext()
	want.Label = "Renamed"
	want.CodexEdition = &amp.Address{
		PlanetID_0: 0xED,
	}
	want.SetCryptoKitID(safe.Crypto.P256.ID)
	got := nextTerms(t, rotationPrev(), amp.TermsChange{
		Template: template,
	})
	assertTerms(t, "set terms", got, want)

	prev := rotationPrev()
	prev.CodexEdition = edition
	assertTerms(t, "CodexEdition never carries", nextTerms(t, prev, amp.TermsChange{}), rotationNext())

	for _, refused := range []*amp.EpochTerms{
		{LicenseClass: tagWithUID(0x34, 0x34)},
		{ContentSealedSince: tagWithUID(0x35, 0x35)},
	} {
		_, err := amp.NextEpochTerms(rotationPrev(), amp.TermsChange{
			Template: refused,
			EpochTag: successorTag(),
		})
		if !errors.Is(err, amp.ErrTermsTemplate) {
			t.Errorf("template %v: err = %v, want ErrTermsTemplate", refused, err)
		}
	}
}

// IntroduceSealedSince sets the marker to the new EpochTag only when the
// predecessor carries none.
func TestNextEpochTerms_IntroduceSealedSince(t *testing.T) {
	assertTerms(t, "a carried marker stands",
		nextTerms(t, rotationPrev(), amp.TermsChange{IntroduceSealedSince: true}), rotationNext())

	prev := rotationPrev()
	prev.ContentSealedSince = nil
	want := rotationNext()
	want.ContentSealedSince = successorTag()
	assertTerms(t, "introduced",
		nextTerms(t, prev, amp.TermsChange{IntroduceSealedSince: true}), want)
}

// The successor of goldenEpoch, assembled, is the golden Terms
// with CodexEdition cleared and the chain advanced; every other term — the
// Public Charter's marker and Seal Paused included — equal.  want is
// goldenLayers' literal with those changes written here.
func TestNextEpochTerms_GoldenSuccessor(t *testing.T) {
	golden := goldenEpoch(t)
	prevTerms, err := golden.ParsedTerms()
	if err != nil {
		t.Fatal(err)
	}
	next := nextTerms(t, prevTerms, amp.TermsChange{})
	successor, err := amp.AssembleSuccessor(golden, next)
	if err != nil {
		t.Fatal(err)
	}
	got, err := successor.ParsedTerms()
	if err != nil {
		t.Fatal(err)
	}
	_, want := goldenLayers()
	want.EpochTag = successorTag()
	want.PreviousEpoch = tagWithUID(0xAAAA, 0xBBBB)
	want.EpochHeight = 8
	want.CodexEdition = nil
	want.CharterHash = fixtureBytes(t, goldenCharterHashHex)
	assertTerms(t, "golden successor", got, want)
	if !bytes.Equal(successor.Charter, golden.Charter) {
		t.Error("successor Charter bytes differ from the golden's")
	}
	if err := successor.VerifySuccessor(golden, amp.TermsChange{}); err != nil {
		t.Errorf("VerifySuccessor: %v", err)
	}
}

// Genesis Terms derive through the same function over no
// predecessor, and the result assembles to the canonical genesis FRAME.
func TestNextEpochTerms_Genesis(t *testing.T) {
	charter, want := genesisLayers()
	template := &amp.EpochTerms{
		Label:       want.Label,
		Foyer:       want.Foyer,
		VaultConfig: want.VaultConfig,
	}
	template.SetCryptoKitID(safe.Crypto.Poly25519.ID)
	got, err := amp.NextEpochTerms(nil, amp.TermsChange{
		Template:             template,
		EpochTag:             want.EpochTag,
		IntroduceSealedSince: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertTerms(t, "genesis Terms", got, want)
	genesis, err := amp.AssembleEpoch(charter, got, safe.HashKitID_Blake2s_256)
	if err != nil {
		t.Fatal(err)
	}
	assertFrame(t, genesis, genesisFrameHex)
}

// GREASE: golden Terms carrying one unknown field in each band
// a successor survives — 77 (carry) and 777 (epoch-local) — rotate twice: 77
// rides every successor, 777 none, and each verifies against its
// predecessor.
func TestNextEpochTerms_Grease(t *testing.T) {
	charter, terms := goldenLayers()
	withUnknown(terms, 77, "grease-carry")
	withUnknown(terms, 777, "grease-local")
	prev, err := amp.AssembleEpoch(charter, terms, safe.HashKitID_Blake2s_256)
	if err != nil {
		t.Fatal(err)
	}
	for height := uint64(8); height <= 9; height++ {
		prevTerms, err := prev.ParsedTerms()
		if err != nil {
			t.Fatal(err)
		}
		next, err := amp.NextEpochTerms(prevTerms, amp.TermsChange{
			EpochTag: tagWithUID(0x6E, height),
		})
		if err != nil {
			t.Fatalf("height %d: %v", height, err)
		}
		successor, err := amp.AssembleSuccessor(prev, next)
		if err != nil {
			t.Fatal(err)
		}
		if err := successor.VerifySuccessor(prev, amp.TermsChange{}); err != nil {
			t.Fatalf("height %d: VerifySuccessor: %v", height, err)
		}
		got, err := successor.ParsedTerms()
		if err != nil {
			t.Fatal(err)
		}
		if nums := unknownNumbers(t, got); !slices.Equal(nums, []protowire.Number{77}) {
			t.Errorf("height %d: unknown fields %v, want [77]", height, nums)
		}
		prev = successor
	}
}

// A Charter stored in a valid non-Go field order keeps
// its bytes across a successor, so continuity holds; a re-marshal would
// reorder them and every rotation would refuse itself as a charter swap.
func TestAssembleSuccessor_CopiesStoredCharter(t *testing.T) {
	planet := tagWithUID(0x7C, 0x01)
	genesisTag := tagWithUID(0x7C, 0x02)
	goCharter := &amp.PlanetCharter{
		CharterSchema: 1,
		PlanetID:      planet,
		GenesisEpoch:  genesisTag,
		Privacy:       amp.PrivacyMode_Public,
	}
	privacy, err := proto.Marshal(&amp.PlanetCharter{
		Privacy: amp.PrivacyMode_Public,
	})
	if err != nil {
		t.Fatal(err)
	}
	rest, err := proto.Marshal(&amp.PlanetCharter{
		CharterSchema: 1,
		PlanetID:      planet,
		GenesisEpoch:  genesisTag,
	})
	if err != nil {
		t.Fatal(err)
	}
	reordered := append(privacy, rest...)
	parsed := &amp.PlanetCharter{}
	if err := proto.Unmarshal(reordered, parsed); err != nil || !proto.Equal(parsed, goCharter) {
		t.Fatalf("setup: the reordered Charter is not the same message (%v)", err)
	}
	if goBytes, _ := proto.Marshal(goCharter); bytes.Equal(goBytes, reordered) {
		t.Fatal("setup: the reordered Charter is in Go's field order")
	}

	kit, err := safe.NewHashKit(safe.HashKitID_Blake2s_256)
	if err != nil {
		t.Fatal(err)
	}
	kit.Hasher.Write(reordered)
	genesisTerms := &amp.EpochTerms{
		TermsSchema: 1,
		EpochTag:    genesisTag,
		CharterHash: kit.Hasher.Sum(nil),
	}
	termsBytes, err := proto.Marshal(genesisTerms)
	if err != nil {
		t.Fatal(err)
	}
	genesis := &amp.PlanetEpoch{
		Charter:  reordered,
		Terms:    termsBytes,
		EpochTag: genesisTag,
	}
	if err := genesis.VerifyGenesis(planet.UID()); err != nil {
		t.Fatalf("setup: genesis: %v", err)
	}

	next := nextTerms(t, genesisTerms, amp.TermsChange{})
	successor, err := amp.AssembleSuccessor(genesis, next)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(successor.Charter, reordered) {
		t.Error("successor Charter bytes are not the stored bytes")
	}
	if err := successor.VerifyCharterContinuity(genesis); err != nil {
		t.Errorf("continuity: %v", err)
	}
	if err := successor.VerifySuccessor(genesis, amp.TermsChange{}); err != nil {
		t.Errorf("VerifySuccessor: %v", err)
	}
}

// genesisEpoch assembles genesisLayers under the default kit.
func genesisEpoch(t *testing.T) *amp.PlanetEpoch {
	t.Helper()
	charter, terms := genesisLayers()
	genesis, err := amp.AssembleEpoch(charter, terms, safe.HashKitID_Blake2s_256)
	if err != nil {
		t.Fatal(err)
	}
	return genesis
}

// successorOf derives and assembles prev's successor under change.
func successorOf(t *testing.T, prev *amp.PlanetEpoch, change amp.TermsChange) *amp.PlanetEpoch {
	t.Helper()
	prevTerms, err := prev.ParsedTerms()
	if err != nil {
		t.Fatal(err)
	}
	successor, err := amp.AssembleSuccessor(prev, nextTerms(t, prevTerms, change))
	if err != nil {
		t.Fatal(err)
	}
	return successor
}

// VerifySuccessor passes the honest successor and refuses Terms beyond the
// change (naming the term), another predecessor, a changed Charter, and a
// predecessor carrying a must-understand field this binary lacks.
func TestVerifySuccessor_Refusals(t *testing.T) {
	genesis := genesisEpoch(t)
	renamed := amp.TermsChange{
		Template: &amp.EpochTerms{
			Label: "Renamed",
		},
	}
	successor := successorOf(t, genesis, renamed)
	if err := successor.VerifySuccessor(genesis, renamed); err != nil {
		t.Fatalf("honest successor: %v", err)
	}

	err := successor.VerifySuccessor(genesis, amp.TermsChange{})
	if err == nil || !strings.Contains(err.Error(), "Label") {
		t.Errorf("Terms beyond the change: err = %v, want a refusal naming Label", err)
	}
	if err := successor.VerifySuccessor(goldenEpoch(t), renamed); err == nil {
		t.Error("another predecessor: accepted")
	}

	charter, _ := genesisLayers()
	charter.Declaration.Head.Text = "Another Compact"
	terms, err := successor.ParsedTerms()
	if err != nil {
		t.Fatal(err)
	}
	swapped, err := amp.AssembleEpoch(charter, terms, safe.HashKitID_Blake2s_256)
	if err != nil {
		t.Fatal(err)
	}
	if err := swapped.VerifySuccessor(genesis, renamed); err == nil {
		t.Error("changed Charter: accepted")
	}

	charter, prevTerms := genesisLayers()
	withUnknown(prevTerms, 1540, "must")
	prev, err := amp.AssembleEpoch(charter, prevTerms, safe.HashKitID_Blake2s_256)
	if err != nil {
		t.Fatal(err)
	}
	newerTerms := proto.Clone(prevTerms).(*amp.EpochTerms)
	newerTerms.EpochTag = successorTag()
	newerTerms.PreviousEpoch = prevTerms.EpochTag
	newerTerms.EpochHeight = 1
	newer, err := amp.AssembleSuccessor(prev, newerTerms)
	if err != nil {
		t.Fatal(err)
	}
	err = newer.VerifySuccessor(prev, amp.TermsChange{})
	if !errors.Is(err, amp.ErrTermsMustUnderstand) || !strings.Contains(err.Error(), "1540") {
		t.Errorf("must-understand predecessor: err = %v, want ErrTermsMustUnderstand naming 1540", err)
	}
}

// ── Mixed-version matrix ─────────────────────────────────────────────────────

// Fields a newer binary knows and this one does not, one per band.
const (
	newerCarryNum = 40
	newerLocalNum = 540
	newerMustNum  = 1540
)

// newerTermsDescriptor is EpochTerms as a newer binary declares it: every
// field this binary knows plus newerCarryNum, newerLocalNum and newerMustNum.
func newerTermsDescriptor(t *testing.T) protoreflect.MessageDescriptor {
	t.Helper()
	base := protodesc.ToDescriptorProto((&amp.EpochTerms{}).ProtoReflect().Descriptor())
	base.Name = proto.String("EpochTermsNewer")
	for _, spec := range []struct {
		num  int32
		name string
	}{
		{newerCarryNum, "NewerCarry"},
		{newerLocalNum, "NewerLocal"},
		{newerMustNum, "NewerMust"},
	} {
		base.Field = append(base.Field, &descriptorpb.FieldDescriptorProto{
			Name:     proto.String(spec.name),
			Number:   proto.Int32(spec.num),
			Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
			JsonName: proto.String(spec.name),
		})
	}
	file := (&amp.EpochTerms{}).ProtoReflect().Descriptor().ParentFile()
	deps := []string{file.Path()}
	for i := 0; i < file.Imports().Len(); i++ {
		deps = append(deps, file.Imports().Get(i).Path())
	}
	fd, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name:        proto.String("amp_test/epoch_terms_newer.proto"),
		Package:     proto.String("amp_test_newer"),
		Dependency:  deps,
		MessageType: []*descriptorpb.DescriptorProto{base},
		Syntax:      proto.String("proto3"),
	}, protoregistry.GlobalFiles)
	if err != nil {
		t.Fatalf("newer descriptor: %v", err)
	}
	return fd.Messages().Get(0)
}

// asNewer reads msg's wire bytes as the newer binary does.
func asNewer(t *testing.T, md protoreflect.MessageDescriptor, msg proto.Message) *dynamicpb.Message {
	t.Helper()
	wire, err := proto.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	newer := dynamicpb.NewMessage(md)
	if err := proto.Unmarshal(wire, newer); err != nil {
		t.Fatal(err)
	}
	return newer
}

// TestNextEpochTerms_MixedVersionMatrix: this binary authors the successor of
// Terms a newer binary wrote, and the newer binary reads the result.  The
// newer binary's own rule, written by hand: 40 carries, 540 drops, 1540 is
// derived — so this binary must match on 40 and 540 and refuse on 1540,
// naming it.  A newer client's template setting 40 refuses here.
func TestNextEpochTerms_MixedVersionMatrix(t *testing.T) {
	md := newerTermsDescriptor(t)
	carryFD := md.Fields().ByNumber(newerCarryNum)
	cases := []struct {
		name    string
		num     protowire.Number
		refuses bool
	}{
		{"no newer field", 0, false},
		{"carry band 40", newerCarryNum, false},
		{"epoch-local 540", newerLocalNum, false},
		{"must-understand 1540", newerMustNum, true},
	}
	for _, c := range cases {
		prev := rotationPrev()
		if c.num != 0 {
			withUnknown(prev, c.num, "v0")
		}
		next, err := amp.NextEpochTerms(prev, amp.TermsChange{
			EpochTag: successorTag(),
		})
		if c.refuses {
			if !errors.Is(err, amp.ErrTermsMustUnderstand) || !strings.Contains(err.Error(), fmt.Sprint(c.num)) {
				t.Errorf("%s: err = %v, want a refusal naming %d", c.name, err, c.num)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		want := asNewer(t, md, rotationNext())
		if c.num == newerCarryNum {
			want.Set(carryFD, protoreflect.ValueOfString("v0"))
		}
		if got := asNewer(t, md, next); !proto.Equal(got, want) {
			t.Errorf("%s: the newer binary reads\n got: %v\nwant: %v", c.name, got, want)
		}
	}

	template := &amp.EpochTerms{}
	withUnknown(template, newerCarryNum, "set-by-a-newer-client")
	_, err := amp.NextEpochTerms(rotationPrev(), amp.TermsChange{
		Template: template,
		EpochTag: successorTag(),
	})
	if !errors.Is(err, amp.ErrTermsTemplate) || !strings.Contains(err.Error(), "40") {
		t.Errorf("newer template: err = %v, want a refusal naming 40", err)
	}
}
