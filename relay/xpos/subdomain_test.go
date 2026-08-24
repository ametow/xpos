package xpos

import (
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/ametow/xpos/events"
	"github.com/ametow/xpos/relay/auth"
)

type subdomainAuthenticator struct{}

func (subdomainAuthenticator) Authenticate(string) (auth.User, error) {
	return auth.User{Login: "AdaDev"}, nil
}

func TestResolveSubdomain(t *testing.T) {
	tests := []struct {
		name      string
		username  string
		requested string
		want      string
	}{
		{name: "defaults to username", username: "AdaDev", want: "adadev"},
		{name: "custom", username: "adadev", requested: "my-demo", want: "my-demo"},
		{name: "normalizes case", username: "adadev", requested: "My-Demo", want: "my-demo"},
		{name: "single character", username: "adadev", requested: "x", want: "x"},
		{name: "maximum length", username: "adadev", requested: strings.Repeat("a", 63), want: strings.Repeat("a", 63)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveSubdomain(tt.username, tt.requested)
			if err != nil {
				t.Fatalf("resolveSubdomain() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("resolveSubdomain() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHandleEventServerRejectsBusyCustomSubdomain(t *testing.T) {
	x := &Xpos{
		hostname:      "xpos.test",
		httpTunnels:   &sync.Map{},
		authenticator: subdomainAuthenticator{},
	}
	x.httpTunnels.Store("my-demo.xpos.test", struct{}{})

	message := requestTunnelError(t, x, "http", "My-Demo")
	if !strings.Contains(message, "subdomain is busy: my-demo") {
		t.Fatalf("error message = %q", message)
	}
}

func TestHandleEventServerRejectsInvalidCustomSubdomain(t *testing.T) {
	x := &Xpos{
		hostname:      "xpos.test",
		httpTunnels:   &sync.Map{},
		authenticator: subdomainAuthenticator{},
	}

	message := requestTunnelError(t, x, "http", "bad_name")
	if !strings.Contains(message, "invalid subdomain") {
		t.Fatalf("error message = %q", message)
	}
}

func TestHandleEventServerRejectsSubdomainForTCP(t *testing.T) {
	x := &Xpos{httpTunnels: &sync.Map{}, authenticator: subdomainAuthenticator{}}
	message := requestTunnelError(t, x, "tcp", "my-demo")
	if !strings.Contains(message, "only supported for http") {
		t.Fatalf("error message = %q", message)
	}
}

func requestTunnelError(t *testing.T, x *Xpos, protocol, subdomain string) string {
	t.Helper()
	client, relay := net.Pipe()
	defer client.Close()

	done := make(chan error, 1)
	go func() { done <- x.handleEventServer(relay) }()

	request := events.NewTunnelRequestEvent()
	request.Data.Protocol = protocol
	request.Data.AuthToken = "token"
	request.Data.Subdomain = subdomain
	if err := request.Write(client); err != nil {
		t.Fatal(err)
	}

	response := events.NewTunnelCreatedEvent()
	if err := response.Read(client); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err == nil {
		t.Fatal("handleEventServer() error = nil")
	}
	return response.Data.ErrorMessage
}

func TestResolveSubdomainRejectsInvalidNames(t *testing.T) {
	for _, requested := range []string{
		"-demo",
		"demo-",
		"my_demo",
		"my.demo",
		"two words",
		strings.Repeat("a", 64),
		"ğ",
	} {
		t.Run(requested, func(t *testing.T) {
			if _, err := resolveSubdomain("user", requested); err == nil {
				t.Fatalf("resolveSubdomain(%q) error = nil", requested)
			}
		})
	}
}
