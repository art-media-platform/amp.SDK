package tag_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/art-media-platform/amp.SDK/stdlib/tag"
)

func TestUIDJSONDecodedString(t *testing.T) {
	wireUID := tag.HashName("json.escaped.uid").ID.Base32()
	tests := []struct {
		name string
		wire string
		text string
	}{
		{
			name: "escaped UID digit",
			wire: fmt.Sprintf(`"\u%04X%s"`, wireUID[0], wireUID[1:]),
			text: wireUID,
		},
		{
			name: "escaped whitespace in name",
			wire: `"hello\u0020world"`,
			text: "hello world",
		},
		{
			name: "quoted name",
			wire: `"hello\"world"`,
			text: `hello"world`,
		},
		{
			name: "escaped slash in URL",
			wire: `"https:\/\/example.com\/Path"`,
			text: "https://example.com/Path",
		},
		{
			name: "surrogate pair",
			wire: `"\ud83c\udf0d"`,
			text: "🌍",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := tag.UID{}
			if err := json.Unmarshal([]byte(test.wire), &got); err != nil {
				t.Fatal(err)
			}
			if want := tag.ParseUID(test.text); got != want {
				t.Fatalf("received %s, want %s from decoded %q", got, want, test.text)
			}
		})
	}
}

func TestUIDJSONRejectsInvalidString(t *testing.T) {
	initial := tag.HashName("preserved.on.error").ID
	for _, wire := range []string{`"bad\q"`, "\"raw\nnewline\""} {
		got := initial
		if err := got.UnmarshalJSON([]byte(wire)); err == nil {
			t.Errorf("accepted invalid JSON string %q", wire)
		}
		if got != initial {
			t.Errorf("invalid JSON %q changed the destination", wire)
		}
	}
}
