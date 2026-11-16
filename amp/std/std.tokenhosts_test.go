package std

import (
	"fmt"
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// ParseTokenHosts: each entry is host or host:port in the field's grammar, the
// port defaults to 443, and one entry outside the grammar refuses the list.
func TestParseTokenHosts(t *testing.T) {
	got, err := ParseTokenHosts([]string{
		"stream.example.org",
		"api.example.org:8443",
		"xn--bcher-kva.example",
		"192.0.2.7",
		"127.0.0.1", // parses; the public-address rule refuses it at dial
		"[2001:db8::1]",
		"[2001:db8::1]:8443",
		"stream.example.org:443", // the same destination as the first
	})
	if err != nil {
		t.Fatalf("a list inside the grammar: %v", err)
	}
	want := []string{
		"stream.example.org:443",
		"api.example.org:8443",
		"xn--bcher-kva.example:443",
		"192.0.2.7:443",
		"127.0.0.1:443",
		"[2001:db8::1]:443",
		"[2001:db8::1]:8443",
	}
	if len(got) != len(want) {
		t.Errorf("the set = %v, want %v", got, want)
	}
	for _, key := range want {
		if !got[key] {
			t.Errorf("the set lacks %s: %v", key, got)
		}
	}
	if none, err := ParseTokenHosts(nil); err != nil || len(none) != 0 {
		t.Errorf("no entries: %v, %v; want an empty set", none, err)
	}

	refused := []string{
		"",
		" stream.example.org",
		"Stream.Example.org",          // not lower case
		"https://stream.example.org",  // a scheme
		"stream.example.org/mp3_high", // a path
		"user@stream.example.org",     // userinfo
		"*.example.org",               // a wildcard
		"stream.example.org.",         // a trailing dot
		"stream..example.org",
		"-stream.example.org",
		"bücher.example",      // an IDN outside its ASCII form
		"stream.example.org:", // an empty port
		"stream.example.org:0",
		"stream.example.org:0443",
		"stream.example.org:65536",
		"stream.example.org:https",
		"stream.example.org:443:443",
		"2001:db8::1",          // IPv6 outside brackets
		"[2001:DB8::1]",        // not its canonical text
		"[2001:db8:0:0::1]",    // not its canonical text
		"[fe80::1%en0]",        // a zone
		"[192.0.2.7]",          // brackets are IPv6's
		"[2001:db8::1]8443",    // no colon ahead of the port
		"stream.example.org?x", // a query
		"stream.example.org#x",
		strings.Repeat("a", 64) + ".example.org", // a label over 63 bytes
		"2130706433",                             // an IPv4 address as one decimal number
		"0x7f.0.0.1",                             // an IPv4 address with a hex part
		"0x7f000001",                             // an IPv4 address as one hex number
		"127.000.000.001",                        // an IPv4 address with leading zeros
		"127.1",                                  // an IPv4 address in short form
		"192.0.2.07",                             // a leading zero in one part
		"1.2.3.4.5",                              // numeric labels that are no IPv4 address
		"example.123",                            // a numeric last label
	}
	for _, entry := range refused {
		// One entry outside the grammar refuses the whole list.
		set, err := ParseTokenHosts([]string{"stream.example.org", entry})
		if err == nil || set != nil {
			t.Errorf("%q: set = %v, err = %v; want the list refused", entry, set, err)
		}
	}
}

// ParseTokenHosts takes at most TokenHostsDefaultMax entries; a longer list is
// refused whole, so the writer and the reader of a record agree.
func TestParseTokenHostsCountBound(t *testing.T) {
	entries := make([]string, 0, TokenHostsDefaultMax+1)
	for i := 0; i < int(TokenHostsDefaultMax); i++ {
		entries = append(entries, fmt.Sprintf("stream%d.example.org", i))
	}
	if set, err := ParseTokenHosts(entries); err != nil || len(set) != int(TokenHostsDefaultMax) {
		t.Errorf("%d entries: %d hosts, err = %v; want all of them", len(entries), len(set), err)
	}
	entries = append(entries, "one-more.example.org")
	if set, err := ParseTokenHosts(entries); err == nil || set != nil {
		t.Errorf("%d entries: set = %d hosts, err = %v; want the list refused", len(entries), len(set), err)
	}
}

// Credentials is sealed on disk: its field numbers never move.
func TestCredentialsFieldNumbers(t *testing.T) {
	fields := (&Credentials{}).ProtoReflect().Descriptor().Fields()
	want := map[string]int{
		"User":       1,
		"Secret":     2,
		"Endpoint":   3,
		"TokenHosts": 4,
	}
	if fields.Len() != len(want) {
		t.Errorf("Credentials has %d fields, want %d", fields.Len(), len(want))
	}
	for name, number := range want {
		field := fields.ByName(protoreflect.Name(name))
		if field == nil || int(field.Number()) != number {
			t.Errorf("Credentials.%s: %v, want field %d", name, field, number)
		}
	}
	if hosts := fields.ByName("TokenHosts"); hosts == nil || !hosts.IsList() || hosts.Kind() != protoreflect.StringKind {
		t.Error("Credentials.TokenHosts is not a repeated string")
	}
}
