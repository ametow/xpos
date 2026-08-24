package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthenticate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("Authorization"), "token gho_secret"; got != want {
			t.Errorf("Authorization = %q, want %q", got, want)
		}
		_, _ = w.Write([]byte(`{"id":42,"name":"Ada","login":"AdaDev","allowed":true}`))
	}))
	defer server.Close()

	user, err := (github{}).authenticate(server.URL, "secret")
	if err != nil {
		t.Fatalf("authenticate() error = %v", err)
	}
	if user.ID != 42 || user.Login != "adadev" || !user.Allowed {
		t.Fatalf("authenticate() user = %#v", user)
	}
}

func TestAuthenticateRejectsNonOKResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := (github{}).authenticate(server.URL, "bad-token")
	if err == nil || !strings.Contains(err.Error(), "invalid token") {
		t.Fatalf("authenticate() error = %v, want invalid token", err)
	}
}

func TestAuthenticateRejectsInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("{"))
	}))
	defer server.Close()

	_, err := (github{}).authenticate(server.URL, "secret")
	if err == nil || !strings.Contains(err.Error(), "failed to decode") {
		t.Fatalf("authenticate() error = %v, want decode error", err)
	}
}
