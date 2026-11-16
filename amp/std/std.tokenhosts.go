package std

import (
	"fmt"
	"net"
	"net/netip"
	"strings"
)

// tokenHostDefaultPort is the port of an entry that names none: https's.
const tokenHostDefaultPort = "443"

// ParseTokenHosts is the one parser of Credentials.TokenHosts, shared by the
// writer of a record and its reader: it returns the set the entries name, each
// as "host:port" (net.JoinHostPort), or an error naming the first entry outside
// the grammar the field states, or a list over TokenHostsDefaultMax entries.  A
// reader that gets an error refuses the record.
func ParseTokenHosts(entries []string) (map[string]bool, error) {
	if len(entries) > int(TokenHostsDefaultMax) {
		return nil, fmt.Errorf("std: %d token hosts, over the %d entry bound", len(entries), TokenHostsDefaultMax)
	}
	hosts := make(map[string]bool, len(entries))
	for _, entry := range entries {
		key, err := parseTokenHost(entry)
		if err != nil {
			return nil, fmt.Errorf("std: token host %q: %w", entry, err)
		}
		hosts[key] = true
	}
	return hosts, nil
}

// parseTokenHost reads one entry: `host` or `host:port`, where host is a
// lower-case DNS name or IPv4 literal, or a bracketed IPv6 literal.
func parseTokenHost(entry string) (string, error) {
	host, port := entry, tokenHostDefaultPort
	if literal, isIPv6 := strings.CutPrefix(entry, "["); isIPv6 {
		address, after, closed := strings.Cut(literal, "]")
		if !closed {
			return "", fmt.Errorf("an IPv6 literal closes its bracket")
		}
		parsed, err := netip.ParseAddr(address)
		if err != nil || !parsed.Is6() || parsed.Zone() != "" || parsed.String() != address {
			return "", fmt.Errorf("brackets hold an IPv6 address in its canonical text")
		}
		host = address
		if after != "" {
			given, hasPort := strings.CutPrefix(after, ":")
			if !hasPort {
				return "", fmt.Errorf("only :port follows an IPv6 literal")
			}
			port = given
		}
	} else {
		if at := strings.LastIndexByte(entry, ':'); at >= 0 {
			host, port = entry[:at], entry[at+1:]
		}
		if !isTokenHostName(host) {
			return "", fmt.Errorf("the host is a lower-case DNS name or an IP literal, with no scheme, path, userinfo or wildcard")
		}
	}
	if !isTokenHostPort(port) {
		return "", fmt.Errorf("the port is 1–65535 in plain digits")
	}
	return net.JoinHostPort(host, port), nil
}

// isTokenHostName reports whether host is dot-separated labels of a–z, 0–9 and
// inner hyphens, each 1–63 bytes, 253 in all: a DNS name in lower case (an IDN
// in its ASCII form) or an IPv4 literal.
func isTokenHostName(host string) bool {
	if host == "" || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			char := label[i]
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
				return false
			}
		}
	}
	return true
}

// isTokenHostPort reports whether port is 1–65535 in digits with no leading zero.
func isTokenHostPort(port string) bool {
	if port == "" || len(port) > 5 || port[0] == '0' {
		return false
	}
	number := 0
	for i := 0; i < len(port); i++ {
		if port[i] < '0' || port[i] > '9' {
			return false
		}
		number = number*10 + int(port[i]-'0')
	}
	return number <= 65535
}
