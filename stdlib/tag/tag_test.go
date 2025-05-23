package tag_test

import (
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

func TestTag(t *testing.T) {
	ampTags := tag.Expr{}.With("..amp+.app.")
	if ampTags.ID != tag.UID_FromExpr(".amp...").WithString("app") {
		t.Fatalf("tag.Expr{}.With().ID failed: %v", ampTags.ID)
	}
	expr := ampTags.With("some-tag+thing")
	if expr.Canonic != "amp.app.some-tag.thing" {
		t.Errorf("With() failed: got %q", expr.Canonic)
	}
	if expr.ID != ampTags.ID.WithExpr("some-tag").WithString("thing") {
		t.Fatalf("WithExpr/WithToken failed: %v", expr.ID)
	}
	base32 := expr.ID.Base32()
	if base32 != "2EDE3DJ7NX5VVY0NXJJRJ161SW" {
		t.Fatalf("tag.UID.Base32() failed: got %v", base32)
	}
	exprID, err := tag.UID_Parse(base32)
	if err != nil || exprID != expr.ID {
		t.Fatalf("UID_Parse(Base32) failed: got %v, err=%v", exprID, err)
	}
	if base16 := expr.ID.Base16(); base16 != "0x4D6346C89E9D2EF7E053B18DE213071C" {
		t.Fatalf("tag.UID.Base16() failed: got %v", base16)
	}
	if prefix, suffix := expr.LeafTags(2); prefix != "amp.app.some" || suffix != "-tag.thing" {
		t.Errorf("LeafTags(2) failed: got prefix=%q, suffix=%q", prefix, suffix)
	}
	{
		Genesis := "בְּרֵאשִׁ֖ית בָּרָ֣א אֱלֹהִ֑ים אֵ֥ת הַשָּׁמַ֖יִם וְאֵ֥ת הָאָֽרֶץ"
		holyExpr, err := tag.ParseExpr(Genesis)
		if err != nil {
			t.Fatalf("tag.UID_Parse() failed: %v", err)
		}

		parts := strings.Split(holyExpr.Canonic, ".")
		for range 3773 {
			rand.Shuffle(len(parts), func(i, j int) {
				parts[i], parts[j] = parts[j], parts[i]
			})
			tryExpr := strings.Join(parts, ".")
			try, _ := tag.ParseExpr(tryExpr)
			if try.ID[0] != 0x89605f6cf4aa1bc3 || try.ID[1] != 0x39b7d3fda743a238 {
				t.Fatalf("tag literals commutation test failed: got %v", try)
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
	if tid.AsLabel() != "ECTRRH" {
		t.Errorf("tag.UID.AsLabel() failed: got %q", tid.AsLabel())
	}
	if tid.Base32() != "7RFXVRFXVRFXVJ4E2QG2ECTRRH" {
		t.Errorf("tag.UID.Base32() failed: got %v", tid.Base32())
	}
	if b16 := tid.Base16(); b16 != "0xF777777777777777123456789ABCDEF0" {
		t.Errorf("tag.UID.Base16() failed: got %v", b16)
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
		if prev.CompareTo(&curr) >= 0 {
			t.Errorf("Add() returned a non-increasing value: %v >= %v", prev, curr)
		}
		if curr.Subtract(prev) != delta {
			t.Errorf("Subtract() returned a wrong value: got %v, want %v", curr.Subtract(prev), delta)
		}
	}

	epsilon := tag.UID{0, tag.EntropyMask}

	// Fill prevIDs with new NowID values
	for i := range prevIDs {
		prevIDs[i] = tag.UID_Now()
	}

	// Test for uniqueness and ordering of NowID
	for i := range 10000000 {
		now := tag.UID_Now()
		upperLimit := now.Add(epsilon)

		for _, prev := range prevIDs {
			if prev.CompareTo(&now) == 0 {
				t.Errorf("got duplicate time value")
			}
			comp := prev.CompareTo(&upperLimit)
			if comp >= 0 {
				t.Errorf("got time value outside of epsilon (%v >= %v)", prev, upperLimit)
			}
		}

		prevIDs[i&63] = now
	}
}

func TestIDOps(t *testing.T) {

	id1 := tag.UID{0x123456789abcdef0, 0x123456789abcdef0}
	id2 := tag.UID{0xC876543210fedcba, 0x9876543210fedcba}

	if id1.CompareTo(&id1) != 0 || id1.CompareTo(&id2) >= 0 || id2.CompareTo(&id1) <= 0 {
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
