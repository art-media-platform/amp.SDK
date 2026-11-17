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

// ErrLoginUnavailable is returned by ResolveLoginToken when the endpoint gave no
// answer on the login itself: the request did not complete, the endpoint answered
// a server error, 408 or 429 (LoginStatusError), its response could not be read,
// or its 2xx body is not a JSON object (an overload page).  The login was neither
// accepted nor rejected, so posting it again later is sound.
var ErrLoginUnavailable = status.Code_NotReady.Error("std: login endpoint unavailable")

// LoginStatusError is the answer of a login endpoint whose HTTP status is no
// success, no rejection (ErrLoginRejected) and no redirect (ErrLoginRedirected).
// It is ErrLoginUnavailable for a server error, 408 and 429; any other status
// says the request itself is refused, and posting it again changes nothing.
type LoginStatusError struct {
	Code       int    // the HTTP status
	RetryAfter string // the answer's Retry-After header as sent: delta-seconds or an HTTP-date; "" when none, or longer than either form
}

func (answer *LoginStatusError) Error() string {
	return fmt.Sprintf("std: login endpoint returned HTTP %d", answer.Code)
}

// Is reports ErrLoginUnavailable for a status that another post can outlast.
func (answer *LoginStatusError) Is(target error) bool {
	if target != ErrLoginUnavailable {
		return false
	}
	return answer.Code >= 500 || answer.Code == http.StatusTooManyRequests || answer.Code == http.StatusRequestTimeout
}

// loginUnavailable marks a failure as ErrLoginUnavailable and keeps its text.
type loginUnavailable struct {
	error
}

// unanswered is the failure of a login the endpoint gave no answer on: ctx.Err()
// when the caller's ctx has ended, since the caller left before the endpoint
// was heard from; else err as ErrLoginUnavailable.
func unanswered(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return loginUnavailable{err}
}

func (loginUnavailable) Is(target error) bool { return target == ErrLoginUnavailable }

func (failure loginUnavailable) Unwrap() error { return failure.error }

// retryAfterMaxBytes bounds the Retry-After value a LoginStatusError keeps: an
// HTTP-date is 29 bytes.
const retryAfterMaxBytes = 64

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
// loginResponseMaxBytes.  Any other status that is no success is a
// LoginStatusError, which carries the answer's Retry-After; an endpoint that gave
// no answer on the login is ErrLoginUnavailable.  A ctx that ends before the
// endpoint answers returns ctx.Err(), which is not ErrLoginUnavailable.
func ResolveLoginToken(ctx context.Context, form *LoginForm, user, pass string) (string, error) {
	if form == nil || form.TokenPostURL == "" {
		return "", fmt.Errorf("std: login form has no token endpoint")
	}
	userField, passField, tokenField := form.FieldNames()

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
		return "", unanswered(ctx, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return "", fmt.Errorf("%w (HTTP %d)", ErrLoginRejected, resp.StatusCode)
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return "", fmt.Errorf("%w (HTTP %d)", ErrLoginRedirected, resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		retryAfter := resp.Header.Get("Retry-After")
		if len(retryAfter) > retryAfterMaxBytes {
			retryAfter = ""
		}
		return "", &LoginStatusError{
			Code:       resp.StatusCode,
			RetryAfter: retryAfter,
		}
	}

	answer, err := io.ReadAll(io.LimitReader(resp.Body, loginResponseMaxBytes+1))
	if err != nil {
		return "", unanswered(ctx, fmt.Errorf("std: read login response: %w", err))
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
// string (a non-string value is returned as its raw JSON text).  A body that is
// no JSON object is ErrLoginUnavailable: the endpoint answered with something
// other than its login response.
func decodeJSONField(reader io.Reader, field string) (string, error) {
	object := map[string]json.RawMessage{}
	if err := json.NewDecoder(reader).Decode(&object); err != nil {
		return "", loginUnavailable{fmt.Errorf("std: decode login response: %w", err)}
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

// FieldNames returns the form's request and response field names, each its
// default when unset: the one site of LoginForm's defaults.
func (form *LoginForm) FieldNames() (userField, passField, tokenField string) {
	return orDefault(form.GetUserField(), "username"), orDefault(form.GetPassField(), "password"), orDefault(form.GetTokenField(), "token")
}

// orDefault returns value, or fallback when value is empty.
func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
