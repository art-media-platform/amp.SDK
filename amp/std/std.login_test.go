package std

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// A login endpoint that answers with a redirect is refused: the user and
// password reach the named endpoint and no other origin.
func TestResolveLoginTokenRefusesRedirects(t *testing.T) {
	codes := []int{
		http.StatusMovedPermanently,
		http.StatusFound,
		http.StatusSeeOther,
		http.StatusTemporaryRedirect,
		http.StatusPermanentRedirect,
	}
	for _, code := range codes {
		received := make(chan string, 4)
		elsewhere := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			body, _ := io.ReadAll(request.Body)
			received <- request.Method + " " + request.URL.Path + " " + string(body)
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"token":"stolen"}`))
		}))
		endpoint := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			http.Redirect(writer, request, elsewhere.URL+"/collect", code)
		}))

		form := &LoginForm{TokenPostURL: endpoint.URL + "/v3/auth/token"}
		token, err := ResolveLoginToken(context.Background(), form, "user@example.com", "hunter2")
		endpoint.Close()
		elsewhere.Close()

		select {
		case got := <-received:
			t.Errorf("HTTP %d: the redirect target received a request: %s", code, got)
		default:
		}
		if !errors.Is(err, ErrLoginRedirected) {
			t.Errorf("HTTP %d: err = %v, want ErrLoginRedirected", code, err)
		}
		if token != "" {
			t.Errorf("HTTP %d: token = %q, want none", code, token)
		}
		if err != nil && (strings.Contains(err.Error(), "hunter2") || strings.Contains(err.Error(), elsewhere.URL)) {
			t.Errorf("HTTP %d: the error names the password or the redirect target: %v", code, err)
		}
	}
}

// A login response longer than the bound is refused, not read to its end.
func TestResolveLoginTokenBoundsTheResponse(t *testing.T) {
	endpoint := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"pad":"` + strings.Repeat("x", loginResponseMaxBytes) + `","token":"late"}`))
	}))
	defer endpoint.Close()

	form := &LoginForm{TokenPostURL: endpoint.URL + "/v3/auth/token"}
	token, err := ResolveLoginToken(context.Background(), form, "user@example.com", "hunter2")
	if err == nil || token != "" {
		t.Fatalf("token = %q, err = %v; want a refused over-long response", token, err)
	}
}

// The exchange returns the response's token field.
func TestResolveLoginTokenReturnsTheToken(t *testing.T) {
	endpoint := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"token":"listener-token"}`))
	}))
	defer endpoint.Close()

	form := &LoginForm{TokenPostURL: endpoint.URL + "/v3/auth/token"}
	token, err := ResolveLoginToken(context.Background(), form, "user@example.com", "hunter2")
	if err != nil || token != "listener-token" {
		t.Fatalf("token = %q, err = %v", token, err)
	}
}

// An endpoint that gave no answer on the login is told apart from one that
// refused it: a server error, 408, 429, a 2xx page that is no JSON object and a
// request that did not complete are ErrLoginUnavailable, and none is a
// rejection.  A status error carries its code and the Retry-After the endpoint
// sent, in either form.
func TestResolveLoginTokenTellsUnavailableFromRefused(t *testing.T) {
	httpDate := time.Now().Add(2 * time.Minute).UTC().Format(http.TimeFormat)
	cases := []struct {
		name        string
		code        int
		retryAfter  string
		body        string
		unavailable bool
	}{
		{"overloaded", 529, "120", "", true},
		{"unavailable, HTTP-date", http.StatusServiceUnavailable, httpDate, "", true},
		{"bad gateway", http.StatusBadGateway, "", "", true},
		{"server error", http.StatusInternalServerError, "", "", true},
		{"too many requests", http.StatusTooManyRequests, "30", "", true},
		{"request timeout", http.StatusRequestTimeout, "", "", true},
		{"an over-long Retry-After", http.StatusServiceUnavailable, strings.Repeat("9", retryAfterMaxBytes+1), "", true},
		{"bad request", http.StatusBadRequest, "120", "", false},
		{"not found", http.StatusNotFound, "", "", false},
		{"an overload page at 200", http.StatusOK, "", "<html>503 Service Unavailable</html>", true},
		{"a truncated body at 200", http.StatusOK, "", `{"tok`, true},
		{"a JSON answer with no token", http.StatusOK, "", `{"error":"no such user"}`, false},
	}
	for _, testCase := range cases {
		endpoint := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if testCase.retryAfter != "" {
				writer.Header().Set("Retry-After", testCase.retryAfter)
			}
			writer.WriteHeader(testCase.code)
			_, _ = writer.Write([]byte(testCase.body))
		}))
		form := &LoginForm{TokenPostURL: endpoint.URL + "/v3/auth/token"}
		token, err := ResolveLoginToken(context.Background(), form, "user@example.com", "hunter2")
		endpoint.Close()

		if err == nil || token != "" || errors.Is(err, ErrLoginRejected) {
			t.Errorf("%s: token = %q, err = %v; want an error that is no rejection", testCase.name, token, err)
			continue
		}
		if got := errors.Is(err, ErrLoginUnavailable); got != testCase.unavailable {
			t.Errorf("%s: errors.Is(err, ErrLoginUnavailable) = %v, want %v (%v)", testCase.name, got, testCase.unavailable, err)
		}
		if strings.Contains(err.Error(), "hunter2") {
			t.Errorf("%s: the error names the password: %v", testCase.name, err)
		}
		if testCase.code == http.StatusOK {
			continue
		}
		answer := (*LoginStatusError)(nil)
		wantRetryAfter := testCase.retryAfter
		if len(wantRetryAfter) > retryAfterMaxBytes {
			wantRetryAfter = ""
		}
		if !errors.As(err, &answer) || answer.Code != testCase.code || answer.RetryAfter != wantRetryAfter {
			t.Errorf("%s: err = %#v, want a LoginStatusError of HTTP %d with Retry-After %q", testCase.name, err, testCase.code, wantRetryAfter)
		}
	}

	// A request that does not complete: the endpoint is gone.
	gone := httptest.NewServer(http.NotFoundHandler())
	form := &LoginForm{TokenPostURL: gone.URL + "/v3/auth/token"}
	gone.Close()
	if _, err := ResolveLoginToken(context.Background(), form, "user@example.com", "hunter2"); !errors.Is(err, ErrLoginUnavailable) {
		t.Errorf("a refused connection: err = %v, want ErrLoginUnavailable", err)
	}
}

// A login whose caller leaves while it is in flight returns the caller's
// ctx.Err(): the endpoint was not heard from, so the failure is no
// ErrLoginUnavailable, which would space the login as an endpoint that gave no
// answer.
func TestResolveLoginTokenReturnsTheCallersEnd(t *testing.T) {
	arrived := make(chan struct{})
	endpoint := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body) // the server notices a gone client only once the body is read
		close(arrived)
		<-request.Context().Done()
	}))
	defer endpoint.Close()
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-arrived
		cancel()
	}()
	form := &LoginForm{TokenPostURL: endpoint.URL + "/v3/auth/token"}
	token, err := ResolveLoginToken(ctx, form, "user@example.com", "hunter2")
	if token != "" || err != context.Canceled || errors.Is(err, ErrLoginUnavailable) {
		t.Errorf("token = %q, err = %v; want the caller's context.Canceled, which is no ErrLoginUnavailable", token, err)
	}
}
