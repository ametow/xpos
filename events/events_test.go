package events

import (
	"bytes"
	"strings"
	"testing"
)

func TestRoundTripOpenStream(t *testing.T) {
	var buf bytes.Buffer
	out := NewOpenStreamEvent()
	out.Data.ClientAddr = "203.0.113.5:51234"
	out.Data.InitialData = []byte("GET / HTTP/1.1\r\nHost: x.example.com\r\n\r\n")
	if err := out.Write(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	in := NewOpenStreamEvent()
	if err := in.Read(&buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if in.Data.ClientAddr != out.Data.ClientAddr {
		t.Fatalf("ClientAddr mismatch: %q vs %q", in.Data.ClientAddr, out.Data.ClientAddr)
	}
	if string(in.Data.InitialData) != string(out.Data.InitialData) {
		t.Fatalf("InitialData mismatch")
	}
}

func TestRoundTripTunnelRequest(t *testing.T) {
	var buf bytes.Buffer
	out := NewTunnelRequestEvent()
	out.Data.Protocol = "tcp"
	out.Data.AuthToken = "abc"
	if err := out.Write(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got := buf.Bytes()[0]; got != ProtocolVersion {
		t.Fatalf("first byte = %d, want %d (version)", got, ProtocolVersion)
	}
	if got := EventType(buf.Bytes()[1]); got != EventTypeTunnelRequest {
		t.Fatalf("type byte = %d, want %d", got, EventTypeTunnelRequest)
	}
	in := NewTunnelRequestEvent()
	if err := in.Read(&buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if in.Data.Protocol != "tcp" || in.Data.AuthToken != "abc" {
		t.Fatalf("payload mismatch: %#v", in.Data)
	}
}

func TestRejectsWrongVersion(t *testing.T) {
	var buf bytes.Buffer
	out := NewTunnelRequestEvent()
	out.Data.Protocol = "tcp"
	if err := out.Write(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Corrupt the version byte.
	b := buf.Bytes()
	b[0] = ProtocolVersion + 1

	in := NewTunnelRequestEvent()
	err := in.Read(bytes.NewReader(b))
	if err == nil || !strings.Contains(err.Error(), "unsupported protocol version") {
		t.Fatalf("expected version error, got %v", err)
	}
}

func TestRejectsWrongType(t *testing.T) {
	var buf bytes.Buffer
	out := NewTunnelRequestEvent()
	out.Data.Protocol = "tcp"
	if err := out.Write(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	in := NewTunnelCreatedEvent()
	err := in.Read(&buf)
	if err == nil || !strings.Contains(err.Error(), "unexpected event type") {
		t.Fatalf("expected type error, got %v", err)
	}
}
