package tag_test

import (
	"fmt"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

func TestTag(t *testing.T) {
	amp_tags := tag.Spec{}.With("..amp+.app.")
	if amp_tags.ID != tag.FromString(".amp...").WithToken("app") {
		t.Fatalf("FormSpec.ID failed: %v", amp_tags.ID)
	}
	spec := amp_tags.With("some-tag+thing")
	if spec.Canonic != "amp.app.some-tag.thing" {
		t.Errorf("FormSpec failed")
	}
	if spec.ID != amp_tags.ID.WithToken("some-tag").WithToken("thing") {
		t.Fatalf("FormSpec.ID failed: %v", spec.ID)
	}
	if base32 := spec.ID.Base32(); base32 != "28n1xcwsd1q27g4xus3errv17c4r9ecqx" {
		t.Fatalf("tag.ID.Base32() failed: %v", base32)
	}
	if base16 := spec.ID.Base16(); base16 != "24503d5f30c0d847793bac0db7bec27592e96aedd" {
		t.Fatalf("tag.ID.Base16() failed: %v", base16)
	}
	if prefix, suffix := spec.LeafTags(2); prefix != "amp.app" || suffix != "some-tag.thing" {
		t.Errorf("LeafTags failed")
	}
	genesisStr := "בְּרֵאשִׁ֖ית בָּרָ֣א אֱלֹהִ֑ים אֵ֥ת הַשָּׁמַ֖יִם וְאֵ֥ת הָאָֽרֶץ"
	if id := tag.FromToken(genesisStr); id[0] != 0xaf07b523 && id[1] != 0xe28f0f37f843664a && id[2] != 0x2c445b67f2be39a0 {
		t.Fatalf("tag.FromString() failed: %v", id)
	}
	tid := tag.ID{0x3, 0x7777777777777777, 0x123456789abcdef0}
	if tid.Base32Suffix() != "g2ectrrh" {
		t.Errorf("tag.ID.Base32Suffix() failed")
	}
	if tid.Base32() != "vrfxvrfxvrfxvj4e2qg2ectrrh" {
		t.Errorf("tag.ID.Base32() failed: %v", tid.Base32())
	}
	if b16 := tid.Base16(); b16 != "37777777777777777123456789abcdef0" {
		t.Errorf("tag.ID.Base16() failed: %v", b16)
	}
	if tid.Base16Suffix() != "abcdef0" {
		t.Errorf("tag.ID.Base16Suffix() failed")
	}

	//fmt.Print(tid.FormAsciiBadge())

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
			comp := prev.CompareTo(upperLimit)
			if comp >= 0 {
				t.Errorf("got time value outside of epsilon (%v > %v) ", prev, now)
			}
		}

		prevIDs[i&63] = now
	}
}
