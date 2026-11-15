package std

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/art-media-platform/amp.SDK/stdlib/status"
)

// ErrLoginRejected is returned by ResolveLoginToken when the login endpoint rejects
// the supplied credentials (HTTP 401/403): the username/password is wrong or the
// account lacks access.  Callers match it with errors.Is to report an auth failure
// distinctly from a transport or decode error.
var ErrLoginRejected = status.Code_LoginFailed.Error("std: login rejected (bad credentials or no access)")

// ErrLoginRedirected is returned by ResolveLoginToken when the login endpoint
// answers with a redirect: the user and password go to the form's TokenPostURL
// and nowhere else, so no redirect is followed.
var ErrLoginRedirected = status.Code_LoginFailed.Error("std: login endpoint answered with a redirect; refused")

// loginResponseMaxBytes bounds a login response; a longer one is refused.
const loginResponseMaxBytes = 1 << 20

// loginClient bounds the credential->token exchange in time and follows no
// redirect: the response to the one POST is the answer.
var loginClient = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

// ResolveLoginToken performs a LoginForm's credential->token exchange: it POSTs the
// user + password to form.TokenPostURL (encoded per form.Encoding) and returns the
// form.TokenField value from the JSON response.  The password is used only for this
// request — never stored, logged, or returned.  On HTTP 401/403 it returns a wrapped
// ErrLoginRejected so the caller can report an auth failure precisely.  A redirect
// is never followed (ErrLoginRedirected), and the response is read to at most
// loginResponseMaxBytes.
func ResolveLoginToken(ctx context.Context, form *LoginForm, user, pass string) (string, error) {
	if form == nil || form.TokenPostURL == "" {
		return "", fmt.Errorf("std: login form has no token endpoint")
	}
	userField := orDefault(form.UserField, "username")
	passField := orDefault(form.PassField, "password")
	tokenField := orDefault(form.TokenField, "token")

	var body io.Reader
	contentType := "application/json"
	if form.Encoding == BodyEncoding_Form {
		values := url.Values{userField: {user}, passField: {pass}}
		body = strings.NewReader(values.Encode())
		contentType = "application/x-www-form-urlencoded"
	} else {
		payload, err := json.Marshal(map[string]string{userField: user, passField: pass})
		if err != nil {
			return "", err
		}
		body = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, form.TokenPostURL, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")

	resp, err := loginClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", fmt.Errorf("%w (HTTP %d)", ErrLoginRejected, resp.StatusCode)
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return "", fmt.Errorf("%w (HTTP %d)", ErrLoginRedirected, resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("std: login endpoint returned HTTP %d", resp.StatusCode)
	}

	answer, err := io.ReadAll(io.LimitReader(resp.Body, loginResponseMaxBytes+1))
	if err != nil {
		return "", fmt.Errorf("std: read login response: %w", err)
	}
	if len(answer) > loginResponseMaxBytes {
		return "", fmt.Errorf("std: login response exceeds %d bytes", loginResponseMaxBytes)
	}
	token, err := decodeJSONField(bytes.NewReader(answer), tokenField)
	if err != nil {
		return "", err
	}
	if token == "" {
		return "", fmt.Errorf("std: login response carried no %q token", tokenField)
	}
	return token, nil
}

// decodeJSONField decodes a JSON object and returns the named top-level field as a
// string (a non-string value is returned as its raw JSON text).
func decodeJSONField(reader io.Reader, field string) (string, error) {
	object := map[string]json.RawMessage{}
	if err := json.NewDecoder(reader).Decode(&object); err != nil {
		return "", fmt.Errorf("std: decode login response: %w", err)
	}
	raw, ok := object[field]
	if !ok {
		return "", nil
	}
	asString := ""
	if err := json.Unmarshal(raw, &asString); err == nil {
		return asString, nil
	}
	return strings.Trim(string(raw), `"`), nil
}

// orDefault returns value, or fallback when value is empty.
func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
