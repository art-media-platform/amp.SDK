package std

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
)

// LoginEndpointKey: two URLs name one endpoint when scheme, host, port and path
// agree; a URL that is not absolute https as written, or that carries userinfo,
// a query or a fragment, names none.
func TestLoginEndpointKey(t *testing.T) {
	same := [][2]string{
		{"https://api.example.org/v3/auth/token", "https://api.example.org/v3/auth/token"},
		{"HTTPS://API.Example.org:443/v3/auth/token", "https://api.example.org/v3/auth/token"},
		{"https://api.example.org", "https://api.example.org/"},
	}
	for _, pair := range same {
		first, firstErr := LoginEndpointKey(pair[0], false)
		second, secondErr := LoginEndpointKey(pair[1], false)
		if firstErr != nil || secondErr != nil || first != second {
			t.Errorf("%q and %q should name one endpoint: %q (%v), %q (%v)", pair[0], pair[1], first, firstErr, second, secondErr)
		}
	}
	differ := [][2]string{
		{"https://api.example.org/login", "https://api.example.net/login"},
		{"https://api.example.org/login", "https://api.example.org:8443/login"},
		{"https://api.example.org/login", "https://api.example.org/login/other"},
		{"https://api.example.org/login", "https://api.example.org.evil.test/login"},
	}
	for _, pair := range differ {
		first, firstErr := LoginEndpointKey(pair[0], false)
		second, secondErr := LoginEndpointKey(pair[1], false)
		if firstErr != nil || secondErr != nil || first == second {
			t.Errorf("%q and %q should be two endpoints: %q (%v), %q (%v)", pair[0], pair[1], first, firstErr, second, secondErr)
		}
	}
	refused := []string{
		"",
		"api.example.org/login",
		"ftp://api.example.org/login",
		"http://api.example.org/login",
		"https://api.example.org/login?next=https://elsewhere.test/",
		"https://api.example.org/login?",
		"https://api.example.org/login#frag",
		"https://user:pass@api.example.org/login",
		"https://api.example.org/login ",
		" https://api.example.org/login",
		"https://api.example.org/login\n",
	}
	for _, rawURL := range refused {
		if key, err := LoginEndpointKey(rawURL, false); err == nil {
			t.Errorf("%q is a login endpoint (%q), want a refusal", rawURL, key)
		}
	}
	// allowHTTP admits http, and nothing else that was refused.
	if _, err := LoginEndpointKey("http://127.0.0.1:8080/login", true); err != nil {
		t.Errorf("http under allowHTTP: %v", err)
	}
	if key, err := LoginEndpointKey("http://127.0.0.1:8080/login?x=1", true); err == nil {
		t.Errorf("a query under allowHTTP is a login endpoint (%q)", key)
	}
}

// CheckCredentials returns the exchange a holder posts: the sealed Endpoint
// string itself, in the record's Login shape, each unset field at its default.
func TestCheckCredentials_FormIsTheSealedShape(t *testing.T) {
	const endpoint = "https://API.example.org/v3/x/%2e%2e/auth/token"
	sealed, err := CheckCredentials(&Credentials{
		User:       "listener@example.org",
		Secret:     "s3cret",
		Endpoint:   endpoint,
		TokenHosts: []string{"stream.example.org"},
		Login: &LoginForm{
			Encoding:  BodyEncoding_Form,
			UserField: "email",
		},
	}, false)
	if err != nil {
		t.Fatalf("a record inside the rules: %v", err)
	}
	want := &LoginForm{
		Encoding:     BodyEncoding_Form,
		UserField:    "email",
		TokenPostURL: endpoint,
	}
	if !proto.Equal(sealed.Form, want) {
		t.Errorf("Form = %v, want %v", sealed.Form, want)
	}
	if userField, passField, tokenField := sealed.Form.fieldNames(); userField != "email" || passField != "password" || tokenField != "token" {
		t.Errorf("field names = %q %q %q, want email password token", userField, passField, tokenField)
	}
	if sealed.EndpointKey != "https://api.example.org:443/v3/auth/token" {
		t.Errorf("EndpointKey = %q", sealed.EndpointKey)
	}
	if !sealed.TokenHosts["stream.example.org:443"] || len(sealed.TokenHosts) != 1 {
		t.Errorf("TokenHosts = %v", sealed.TokenHosts)
	}

	// No Login: the shape of every default, at Endpoint.
	sealed, err = CheckCredentials(&Credentials{Endpoint: endpoint}, false)
	if err != nil || !proto.Equal(sealed.Form, &LoginForm{TokenPostURL: endpoint}) {
		t.Errorf("a record with no Login: Form = %v, err = %v", sealed.Form, err)
	}

	// No Endpoint: a secret that is posted nowhere (an API key) has no Form.
	sealed, err = CheckCredentials(&Credentials{Secret: "api-key"}, false)
	if err != nil || sealed.Form != nil || sealed.EndpointKey != "" {
		t.Errorf("a record with no Endpoint: %+v, err = %v", sealed, err)
	}
}

func TestCheckCredentials_Refusals(t *testing.T) {
	const endpoint = "https://api.example.org/login"
	long := strings.Repeat("f", int(LoginFieldsDefaultMaxBytes)+1)
	refused := map[string]*Credentials{
		"an http Endpoint":                    {Endpoint: "http://api.example.org/login"},
		"an Endpoint with a query":            {Endpoint: endpoint + "?next=x"},
		"an Endpoint with a trailing space":   {Endpoint: endpoint + " "},
		"a token host outside the grammar":    {Endpoint: endpoint, TokenHosts: []string{"https://stream.example.org"}},
		"a Login with no Endpoint":            {Login: &LoginForm{UserField: "email"}},
		"a Login that names a URL":            {Endpoint: endpoint, Login: &LoginForm{TokenPostURL: endpoint}},
		"an undefined Encoding":               {Endpoint: endpoint, Login: &LoginForm{Encoding: BodyEncoding(7)}},
		"a UserField over the bound":          {Endpoint: endpoint, Login: &LoginForm{UserField: long}},
		"a PassField over the bound":          {Endpoint: endpoint, Login: &LoginForm{PassField: long}},
		"a TokenField over the bound":         {Endpoint: endpoint, Login: &LoginForm{TokenField: long}},
		"a field name with a control byte":    {Endpoint: endpoint, Login: &LoginForm{UserField: "user\nname"}},
		"one name for user and password":      {Endpoint: endpoint, Login: &LoginForm{UserField: "login", PassField: "login"}},
		"the user under the password default": {Endpoint: endpoint, Login: &LoginForm{UserField: "password"}},
		"the password under the user default": {Endpoint: endpoint, Login: &LoginForm{PassField: "username"}},
	}
	for name, creds := range refused {
		if sealed, err := CheckCredentials(creds, false); err == nil {
			t.Errorf("%s was accepted: %+v", name, sealed)
		}
	}
	atBound := strings.Repeat("f", int(LoginFieldsDefaultMaxBytes))
	if _, err := CheckCredentials(&Credentials{Endpoint: endpoint, Login: &LoginForm{TokenField: atBound}}, false); err != nil {
		t.Errorf("a field name at the bound: %v", err)
	}
	if _, err := CheckCredentials(&Credentials{Endpoint: "http://127.0.0.1:9/login"}, true); err != nil {
		t.Errorf("an http Endpoint under allowHTTP: %v", err)
	}
}
