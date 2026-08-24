package events

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestEventRoundTrip(t *testing.T) {
	var wire bytes.Buffer
	want := NewTunnelRequestEvent()
	want.Data.Protocol = "tcp"
	want.Data.AuthToken = "secret"

	if err := want.Write(&wire); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got := NewTunnelRequestEvent()
	if err := got.Read(&wire); err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if *got.Data != *want.Data {
		t.Fatalf("round trip data = %#v, want %#v", got.Data, want.Data)
	}
}

func TestEventReadRejectsUnexpectedType(t *testing.T) {
	var wire bytes.Buffer
	wire.WriteByte(byte(EventTypeNewConnection))
	_ = binary.Write(&wire, binary.LittleEndian, uint16(2))
	wire.WriteString("{}")

	err := NewTunnelRequestEvent().Read(&wire)
	if err == nil || !strings.Contains(err.Error(), "unexpected event type") {
		t.Fatalf("Read() error = %v, want unexpected event type", err)
	}
}

func TestEventReadErrors(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "short header", data: []byte{byte(EventTypeTunnelRequest)}},
		{name: "short payload", data: []byte{byte(EventTypeTunnelRequest), 5, 0, '{'}},
		{name: "invalid json", data: []byte{byte(EventTypeTunnelRequest), 1, 0, '{'}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := NewTunnelRequestEvent().Read(bytes.NewReader(tt.data)); err == nil {
				t.Fatal("Read() error = nil, want an error")
			}
		})
	}
}

func TestEventWriteRejectsOversizedPayload(t *testing.T) {
	event := NewTunnelCreatedEvent()
	event.Data.ErrorMessage = strings.Repeat("x", maxFrameSize)
	if err := event.Write(io.Discard); err == nil || !strings.Contains(err.Error(), "payload too large") {
		t.Fatalf("Write() error = %v, want payload too large", err)
	}
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

func TestEventWriteDetectsShortWrite(t *testing.T) {
	err := NewConnectionEvent().Write(shortWriter{})
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("Write() error = %v, want %v", err, io.ErrShortWrite)
	}
}

func TestWriteErrorWritesEventAndReturnsMessage(t *testing.T) {
	var wire bytes.Buffer
	err := WriteError(&wire, "invalid %s", "protocol")
	if err == nil || err.Error() != "invalid protocol" {
		t.Fatalf("WriteError() error = %v", err)
	}

	event := NewTunnelCreatedEvent()
	if readErr := event.Read(&wire); readErr != nil {
		t.Fatalf("reading error event: %v", readErr)
	}
	if event.Data.ErrorMessage != "invalid protocol" {
		t.Fatalf("ErrorMessage = %q", event.Data.ErrorMessage)
	}
}

func TestWriteErrorReturnsWriterFailure(t *testing.T) {
	want := errors.New("write failed")
	err := WriteError(errorWriter{want}, "message")
	if !errors.Is(err, want) {
		t.Fatalf("WriteError() error = %v, want %v", err, want)
	}
}

type errorWriter struct{ err error }

func (w errorWriter) Write([]byte) (int, error) { return 0, w.err }
