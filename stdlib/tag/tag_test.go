package tag_test

import (
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

func TestTag(t *testing.T) {
	ampTags := tag.Name{}.With("..amp+.app.")
	if ampTags.Canonic != "amp.app" {
		t.Fatalf("With() canonic failed: got %q", ampTags.Canonic)
	}
	// Invariant: a tag's UID is the atomic hash of its canonic string
	// (order significant — no commutative literal fold).
	if ampTags.ID != tag.UID_HashLiteral([]byte(ampTags.Canonic)) {
		t.Fatalf("ID != HashLiteral(Canonic): %v", ampTags.ID)
	}
	name := ampTags.With("some-tag+thing")
	if name.Canonic != "amp.app.some.tag.thing" {
		t.Errorf("With() failed: got %q", name.Canonic)
	}
	if name.ID != tag.UID_HashLiteral([]byte(name.Canonic)) {
		t.Fatalf("chained ID != HashLiteral(Canonic): %v", name.ID)
	}
	base32 := name.ID.Base32()
	if base32 != "5EEZ7JTVNT1D42251GSU28MQY9" {
		t.Fatalf("tag.UID.Base32() failed: got %v", base32)
	}
	parsed, err := tag.Parse(base32)
	if err != nil || parsed.ID != name.ID {
		t.Fatalf("UID_Parse(Base32) failed: got %v, err=%v", parsed, err)
	}
	if base16 := name.ID.Base16(); base16 != "0xAD6FCF1CEE990B0821142FC68489DBC9" {
		t.Fatalf("tag.UID.Base16() failed: got %v", base16)
	}
	if prefix, suffix := name.LeafTags(2); prefix != "amp.app.some" || suffix != "tag.thing" {
		t.Errorf("LeafTags(2) failed: got prefix=%q, suffix=%q", prefix, suffix)
	}
	{
		Genesis := "בְּרֵאשִׁ֖ית בָּרָ֣א אֱלֹהִ֑ים אֵ֥ת הַשָּׁמַ֖יִם וְאֵ֥ת הָאָֽרֶץ"
		holyExpr := tag.NameFrom(Genesis)
		if holyExpr.ID.Base32() != "3EVFMJWNBJ2WG7QB93XK3ZYR2B" {
			t.Fatalf("tag.NameFrom() failed: got %v", holyExpr.ID.Base32())
		}

		// Order is significant: reordered literals yield a DISTINCT UID (the
		// commutative fold is removed).  Only an identity permutation — a
		// shuffle that happens to reproduce the canonic order — preserves it.
		parts := strings.Split(holyExpr.Canonic, ".")
		for range 3773 {
			rand.Shuffle(len(parts), func(i, j int) {
				parts[i], parts[j] = parts[j], parts[i]
			})
			tryExpr := strings.Join(parts, ".")
			try := tag.NameFrom(tryExpr)
			if tryExpr == holyExpr.Canonic {
				if try.ID != holyExpr.ID {
					t.Fatalf("identity permutation changed UID: got %v", try)
				}
			} else if try.ID == holyExpr.ID {
				t.Fatalf("reordered literals collided with canonic UID (commutativity not removed): %q", tryExpr)
			}
		}
	}

	{
		nowGo := time.Now()
		nowUID := tag.UID_FromTime(nowGo)
		nowTime := nowUID.AsTime()
		if !nowGo.Equal(nowTime) {
			t.Errorf("tag.UID_FromTime().Time() failed: %v != %v", nowGo, nowTime)
		}
	}

	tid := tag.UID{0xF777777777777777, 0x123456789abcdef0}
	if tid.AsLabel() != "7R..TRRH" {
		t.Errorf("tag.UID.AsLabel() failed: got %q", tid.AsLabel())
	}
	if tid.Base32() != "7RFXVRFXVRFXVJ4E2QG2ECTRRH" {
		t.Errorf("tag.UID.Base32() failed: got %v", tid.Base32())
	}
	if b16 := tid.Base16(); b16 != "0xF777777777777777123456789ABCDEF0" {
		t.Errorf("tag.UID.Base16() failed: got %v", b16)
	}
}

func TestNameOrderAndIdentity(t *testing.T) {
	// Plain multi-word names are order-significant (no commutative fold).
	if tag.NameFrom("spaces.plan.tools").ID == tag.NameFrom("tools.plan.spaces").ID {
		t.Fatal("plain-name UID must depend on word order")
	}
	if tag.NameFrom("hello world").ID == tag.NameFrom("world hello").ID {
		t.Fatal("plain-name UID must depend on word order")
	}

	// A single literal hashes atomically — identity unchanged.
	if tag.NameFrom("hello").ID != tag.UID_HashLiteral([]byte("hello")) {
		t.Fatal("single-literal name must equal HashLiteral(word)")
	}

	// scheme:identifier names keep the name part and the identifier part
	// SEPARATE — hash(name) combined with hash(:identifier).  This must never
	// collapse into one atomic hash of the whole canonic string, or persisted
	// wallet / DID identities orphan.
	for _, expr := range []string{
		"eth:0xabcdef1234567890abcdef1234567890abcdef12",
		"did:key:z6MkExample",
		"did:pkh:eip155:1:0xabcdef1234567890abcdef1234567890abcdef12",
		"https://example.com/path",
	} {
		name := tag.NameFrom(expr)
		split := tag.PathStart(name.Canonic)
		if split < 0 {
			t.Fatalf("%q: expected a URL split in canonic %q", expr, name.Canonic)
		}
		want := tag.UID_HashLiteral([]byte(name.Canonic[:split])).With(tag.UID_HashLiteral([]byte(name.Canonic[split:])))
		if name.ID != want {
			t.Fatalf("%q: scheme:identifier UID is not name+identifier combine", expr)
		}
		if name.ID == tag.UID_HashLiteral([]byte(name.Canonic)) {
			t.Fatalf("%q: scheme:identifier UID collapsed to whole-string hash (regression)", expr)
		}
	}
}

func TestNow(t *testing.T) {
	var prevIDs [64]tag.UID

	prevIDs[0] = tag.UID{100, (^uint64(0)) - 500}

	delta := tag.UID{100, 117}
	for i := 1; i < 64; i++ {
		prevIDs[i] = prevIDs[i-1].Add(delta)
	}
	for i := 1; i < 64; i++ {
		prev := prevIDs[i-1]
		curr := prevIDs[i]
		if prev.CompareTo(curr) >= 0 {
			t.Errorf("Add() returned a non-increasing value: %v >= %v", prev, curr)
		}
		if curr.Subtract(prev) != delta {
			t.Errorf("Subtract() returned a wrong value: got %v, want %v", curr.Subtract(prev), delta)
		}
	}

	epsilon := tag.UID{0, tag.EntropyMask}

	// Fill prevIDs with new NowID values
	for i := range prevIDs {
		prevIDs[i] = tag.NowID()
	}

	// Test for uniqueness and ordering of NowID
	const sampleCount = 1024

	for i := range sampleCount {
		now := tag.NowID()
		upper := now.Add(epsilon)

		for _, prev := range prevIDs {
			if prev.CompareTo(upper) >= 0 {
				t.Fatalf("NowID outside epsilon: prev=%v upper=%v", prev, upper)
			}
		}

		prevIDs[i&63] = now
	}
}

func TestIDOps(t *testing.T) {

	id1 := tag.UID{0x123456789abcdef0, 0x123456789abcdef0}
	id2 := tag.UID{0xC876543210fedcba, 0x9876543210fedcba}

	if id1.CompareTo(id1) != 0 || id1.CompareTo(id2) >= 0 || id2.CompareTo(id1) <= 0 {
		t.Errorf("tag.UID.CompareTo() failed: %v >= %v", id1, id2)
	}

	s1 := id1.Add(id2)
	if s1.Subtract(id1) != id2 || s1.Subtract(id2) != id1 {
		t.Errorf("tag.UID.Add() failed: got %v", id1.Add(id2))
	}
	// Test UID Midpoint
	uid1 := tag.UID{0x5000000000000000, 0x0000000000200100}
	uid2 := tag.UID{0xF000000000000001, 0x0000000000000000}
	mid1 := uid1.Midpoint(uid2)
	mid2 := uid2.Midpoint(uid1)
	expectedMid := tag.UID{0xA000000000000000, 0x8000000000100080}
	if mid1 != expectedMid || mid2 != expectedMid {
		t.Errorf("UID.Midpoint() failed: got %v, want %v", mid1, expectedMid)
	}

	// Test Midpoint when both UIDs are equal
	uidEqual := tag.UID{0xABCDEF1234567890, 0x1234567890ABCDEF}
	midEqual := uidEqual.Midpoint(uidEqual)
	if midEqual != uidEqual {
		t.Errorf("UID.Midpoint() with equal UIDs failed: got %v, want %v", midEqual, uidEqual)
	}

}
