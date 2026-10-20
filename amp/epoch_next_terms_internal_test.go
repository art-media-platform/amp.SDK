package amp

import (
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// Build Spec 6: every EpochTerms field this binary knows is held to the band
// rule (ZO-design-conventions §2.8) — 1–34 classified in termsFieldTable, a
// later number in an allocated band, a must-understand field derived — and
// every table entry is a field this binary knows.
func TestEpochTermsBand_EveryFieldClassified(t *testing.T) {
	desc := (&EpochTerms{}).ProtoReflect().Descriptor()
	fields := desc.Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		num := fd.Number()
		_, listed := termsFieldTable[num]
		if num <= 34 && !listed {
			t.Errorf("%s (%d): not in termsFieldTable", fd.Name(), num)
		}
		if num > 34 && listed {
			t.Errorf("%s (%d): the table classifies 1–34 only; later numbers take their band", fd.Name(), num)
		}
		switch termsFieldClass(num) {
		case termsUnallocated:
			t.Errorf("%s (%d): numbered in the unallocated band (≥ %d)", fd.Name(), num, termsBandUnallocated)
		case termsMustUnderstand:
			if _, derived := termsDerivations[num]; !derived {
				t.Errorf("%s (%d): a must-understand field with no derivation", fd.Name(), num)
			}
		}
	}
	for num := range termsFieldTable {
		if fields.ByNumber(num) == nil {
			t.Errorf("termsFieldTable classifies %d, not a field this binary knows", num)
		}
	}
}

// The band edges, and the 1–34 numbers no field holds (C11: the pre-freeze
// 9, 14, 15, 17), which carry.
func TestEpochTermsBand_Edges(t *testing.T) {
	cases := []struct {
		num  protoreflect.FieldNumber
		want termsClass
	}{
		{9, termsCarry},
		{14, termsCarry},
		{33, termsCarryOnly},
		{34, termsPredecessorOnly},
		{35, termsCarry},
		{499, termsCarry},
		{500, termsTemplateOnly},
		{999, termsTemplateOnly},
		{1000, termsMustUnderstand},
		{1999, termsMustUnderstand},
		{2000, termsUnallocated},
	}
	for _, c := range cases {
		if got := termsFieldClass(c.num); got != c.want {
			t.Errorf("field %d: class %d, want %d", c.num, got, c.want)
		}
	}
}
