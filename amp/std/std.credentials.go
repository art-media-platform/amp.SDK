package std

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"path"
	"strings"
)

// SealedLogin is a Credentials record's login fields as CheckCredentials read
// them.
type SealedLogin struct {
	TokenHosts  map[string]bool // ParseTokenHosts of the record's TokenHosts
	EndpointKey string          // LoginEndpointKey of Endpoint; "" when the record names none
	Form        *LoginForm      // the exchange a holder posts: the record's Login shape at Endpoint; nil when the record names none
}

// CheckCredentials is the one check of a Credentials record's login fields,
// shared by the record's writer and its reader; a reader that gets an error
// refuses the record.  It refuses:
//   - TokenHosts that ParseTokenHosts refuses;
//   - an Endpoint that LoginEndpointKey refuses;
//   - a Login with no Endpoint to post it to;
//   - a Login that sets TokenPostURL (Endpoint is the URL), names an Encoding
//     that is not defined, holds a field name over LoginFieldsDefaultMaxBytes
//     or with a control character, or gives the user and the password one name.
//
// The Form it returns posts to the record's Endpoint string itself, in the
// record's Login shape and no other.
func CheckCredentials(creds *Credentials, allowHTTP bool) (SealedLogin, error) {
	sealed := SealedLogin{}
	hosts, err := ParseTokenHosts(creds.GetTokenHosts())
	if err != nil {
		return sealed, err
	}
	sealed.TokenHosts = hosts
	endpoint, shape := creds.GetEndpoint(), creds.GetLogin()
	if endpoint == "" {
		if shape != nil {
			return sealed, errors.New("std: a Login needs the Endpoint it is posted to")
		}
		return sealed, nil
	}
	sealed.EndpointKey, err = LoginEndpointKey(endpoint, allowHTTP)
	if err != nil {
		return sealed, fmt.Errorf("std: Endpoint: %w", err)
	}
	if err := checkLoginShape(shape); err != nil {
		return sealed, fmt.Errorf("std: Login: %w", err)
	}
	sealed.Form = &LoginForm{
		Encoding:     shape.GetEncoding(),
		UserField:    shape.GetUserField(),
		PassField:    shape.GetPassField(),
		TokenPostURL: endpoint,
		TokenField:   shape.GetTokenField(),
	}
	return sealed, nil
}

// checkLoginShape checks the shape a Credentials record seals for its login
// exchange; nil is the shape of every default.
func checkLoginShape(shape *LoginForm) error {
	if shape.GetTokenPostURL() != "" {
		return errors.New("TokenPostURL stays unset: Endpoint is the URL")
	}
	if _, defined := BodyEncoding_name[int32(shape.GetEncoding())]; !defined {
		return fmt.Errorf("Encoding %d is not defined", shape.GetEncoding())
	}
	fields := []struct{ name, value string }{
		{"UserField", shape.GetUserField()},
		{"PassField", shape.GetPassField()},
		{"TokenField", shape.GetTokenField()},
	}
	for _, field := range fields {
		if size := len(field.value); size > int(LoginFieldsDefaultMaxBytes) {
			return fmt.Errorf("%s is %d bytes, over the %d byte bound", field.name, size, LoginFieldsDefaultMaxBytes)
		}
		if strings.ContainsFunc(field.value, isControlRune) {
			return fmt.Errorf("%s holds a control character", field.name)
		}
	}
	if userField, passField, _ := shape.fieldNames(); userField == passField {
		return fmt.Errorf("UserField and PassField are both %q", userField)
	}
	return nil
}

func isControlRune(char rune) bool {
	return char < 0x20 || char == 0x7f
}

// LoginEndpointKey is the one check of a URL a login may be posted to —
// Credentials.Endpoint, and any URL compared with it — and its normalized form:
// scheme, host, port (the scheme's default when omitted) and path.  The URL is
// absolute https, as written (no surrounding space), with no userinfo, query or
// fragment: a query can steer an endpoint, so a URL that carries one names no
// endpoint.  allowHTTP admits http, for a test's fixture server; no production
// caller sets it.  Two URLs name one endpoint when their keys are equal; equal
// keys are not equal requests, so a holder posts to the sealed string and only
// compares any other.
func LoginEndpointKey(rawURL string, allowHTTP bool) (string, error) {
	if strings.TrimSpace(rawURL) != rawURL {
		return "", errors.New("a login URL has no surrounding space")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Opaque != "" {
		return "", errors.New("not an absolute URL of scheme, host and path")
	}
	if parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || parsed.RawFragment != "" {
		return "", errors.New("a login URL carries no query or fragment")
	}
	scheme := strings.ToLower(parsed.Scheme)
	port := parsed.Port()
	switch {
	case scheme == "http" && !allowHTTP, scheme != "https" && scheme != "http":
		return "", errors.New("the login URL is not https")
	case port == "" && scheme == "https":
		port = "443"
	case port == "":
		port = "80"
	}
	return scheme + "://" + net.JoinHostPort(strings.ToLower(parsed.Hostname()), port) + path.Clean("/"+parsed.Path), nil
}
