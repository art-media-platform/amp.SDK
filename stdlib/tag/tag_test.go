package tag_test

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

func TestTag(t *testing.T) {
	amp_tags := tag.Expr{}.With("..amp+.app.")
	if amp_tags.ID != tag.FromExpr(".amp...").WithToken("app") {
		t.Fatalf("FormSpec.ID failed: %v", amp_tags.ID)
	}
	expr := amp_tags.With("some-tag+thing")
	if expr.Canonic != "amp.app.some-tag.thing" {
		t.Errorf("FormSpec failed")
	}
	if expr.ID != amp_tags.ID.WithExpr("some-tag").WithToken("thing") {
		t.Fatalf("FormSpec.ID failed: %v", expr.ID)
	}
	if base32 := expr.ID.Base32(); base32 != "MUFKXXG22D3JUMC38V43HD11CVH57DDD" {
		t.Fatalf("tag.ID.Base32() failed: %v", base32)
	}
	if base16 := expr.ID.Base16(); base16 != "9e9d2ef5e213071d4d6346c83830215ee053b18c" {
		t.Fatalf("tag.ID.Base16() failed: %v", base16)
	}
	if prefix, suffix := expr.LeafTags(2); prefix != "amp.app.some" || suffix != "-tag.thing" {
		t.Errorf("LeafTags failed")
	}
	{
		genesisStr := "בְּרֵאשִׁ֖ית בָּרָ֣א אֱלֹהִ֑ים אֵ֥ת הַשָּׁמַ֖יִם וְאֵ֥ת הָאָֽרֶץ"
		if id := tag.FromToken(genesisStr); id[0] != 0xaf07b523 && id[1] != 0xe28f0f37f843664a && id[2] != 0x2c445b67f2be39a0 {
			t.Fatalf("tag.FromExpr() failed: %v", id)
		}
		parts := strings.Split(genesisStr, " ")
		for i := 0; i < 3773; i++ {
			rand.Shuffle(len(parts), func(i, j int) {
				parts[i], parts[j] = parts[j], parts[i]
			})
			tryExpr := strings.Join(parts, ".")
			tryID := tag.Expr{}.With(tryExpr).ID
			if tryID[0] != 0x2f4aa1bc0 && tryID[1] != 0xa743a23a89605f6a && tryID[2] != 0xcfec4cf239b7d3fa {
				t.Fatalf("tag literals commutation test failed: %v", tryID)
			}
		}
	}

	{
		now := time.Now()
		nowTag := tag.FromTime(now, false)
		nowTime := nowTag.Time()
		if !now.Equal(nowTag.Time()) {
			t.Errorf("tag.FromTime() failed: %v != %v", now, nowTime)
		}
	}

	tid := tag.ID{0x3, 0x7777777777777777, 0x123456789abcdef0}
	if tid.Base32Suffix() != "G2ECTRRH" {
		t.Errorf("tag.ID.Base32Suffix() failed")
	}
	if tid.Base32() != "VRFXVRFXVRFXVJ4E2QG2ECTRRH" {
		t.Errorf("tag.ID.Base32() failed: %v", tid.Base32())
	}
	if b16 := tid.Base16(); b16 != "37777777777777777123456789abcdef0" {
		t.Errorf("tag.ID.Base16() failed: %v", b16)
	}
	if tid.Base16Suffix() != "abcdef0" {
		t.Errorf("tag.ID.Base16Suffix() failed")
	}
}

func TestTagEncodings(t *testing.T) {

	for i := 0; i < 100; i++ {
		id := tag.Now()
		fmt.Println(id.FormAsciiBadge())
	}

}

func TestNewTag(t *testing.T) {
	var prevIDs [64]tag.ID

	prevIDs[0] = tag.ID{100, (^uint64(0)) - 500}

	delta := tag.ID{100, 100}
	for i := 1; i < 64; i++ {
		prevIDs[i] = prevIDs[i-1].Add(delta)
	}
	for i := 1; i < 64; i++ {
		prev := prevIDs[i-1]
		curr := prevIDs[i]
		if prev.CompareTo(curr) >= 0 {
			t.Errorf("tag.ID.Add() returned a non-increasing value: %v <= %v", prev, curr)
		}
		if curr.Sub(prev) != delta {
			t.Errorf("tag.ID.Diff() returned a wrong value: %v != %v", curr.Sub(prev), delta)
		}
	}

	epsilon := tag.ID{0, tag.EntropyMask}

	for i := range prevIDs {
		prevIDs[i] = tag.Now()
	}

	for i := 0; i < 10000000; i++ {
		now := tag.Now()
		upperLimit := now.Add(epsilon)

		for _, prev := range prevIDs {
			if prev.CompareTo(now) == 0 {
				t.Errorf("got duplicate time value")
			}
			comp := prev.CompareTo(upperLimit)
			if comp >= 0 {
				t.Errorf("got time value outside of epsilon (%v > %v) ", prev, now)
			}
		}

		prevIDs[i&63] = now
	}
}
