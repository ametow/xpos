package config

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigWriteAndLoad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"domain":"example.test","events":"127.0.0.1:9876"}`))
	}))
	defer server.Close()

	oldRemote := remoteConfig
	remoteConfig = server.URL
	t.Cleanup(func() { remoteConfig = oldRemote })

	want := Config{}
	want.Local.AuthToken = "token-123"
	if err := want.Write(); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	info, err := os.Stat(filepath.Join(home, localConfig))
	if err != nil {
		t.Fatalf("Stat(config) error = %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("config permissions = %o, want 600", info.Mode().Perm())
	}

	var got Config
	if err := got.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Local.AuthToken != "token-123" || got.Remote.Domain != "example.test" || got.Remote.Events != "127.0.0.1:9876" {
		t.Fatalf("Load() config = %#v", got)
	}
}

func TestConfigLoadWithoutToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var config Config
	err := config.Load()
	if err == nil || !strings.Contains(err.Error(), "no auth token") {
		t.Fatalf("Load() error = %v, want no auth token", err)
	}
}

func TestConfigLoadRemoteFailures(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(home, localConfig), []byte("auth_token: abc\n"), 0600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		body string
		code int
	}{
		{name: "bad status", code: http.StatusBadGateway},
		{name: "bad json", code: http.StatusOK, body: "{"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.code)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			oldRemote := remoteConfig
			remoteConfig = server.URL
			defer func() { remoteConfig = oldRemote }()

			var config Config
			if err := config.Load(); err == nil {
				t.Fatal("Load() error = nil, want an error")
			}
		})
	}
}
