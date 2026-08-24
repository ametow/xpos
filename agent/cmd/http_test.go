package cmd

import "testing"

func TestHTTPCommandSubdomainFlag(t *testing.T) {
	flag := httpCommand.Flags().Lookup("subdomain")
	if flag == nil {
		t.Fatal("http command is missing --subdomain flag")
	}
	if flag.Shorthand != "s" {
		t.Fatalf("subdomain shorthand = %q, want %q", flag.Shorthand, "s")
	}
	if flag.DefValue != "" {
		t.Fatalf("subdomain default = %q, want empty", flag.DefValue)
	}
}

func TestNewTunnelRequestIncludesSubdomain(t *testing.T) {
	request := newTunnelRequest("http", "secret", "my-demo")
	if request.Data.Protocol != "http" || request.Data.AuthToken != "secret" || request.Data.Subdomain != "my-demo" {
		t.Fatalf("tunnel request = %#v", request.Data)
	}
}
