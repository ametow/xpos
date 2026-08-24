package events

import (
	"bytes"
	"testing"
)

func TestTunnelRequestSubdomainRoundTrip(t *testing.T) {
	want := NewTunnelRequestEvent()
	want.Data.Protocol = "http"
	want.Data.AuthToken = "token"
	want.Data.Subdomain = "my-demo"

	var wire bytes.Buffer
	if err := want.Write(&wire); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got := NewTunnelRequestEvent()
	if err := got.Read(&wire); err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if *got.Data != *want.Data {
		t.Fatalf("request = %#v, want %#v", got.Data, want.Data)
	}
}
