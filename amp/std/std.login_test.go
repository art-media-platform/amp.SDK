package std

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
