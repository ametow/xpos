package events

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
)

// Wire format for every event:
//
//	[1 byte type tag][2 bytes little-endian payload length][payload bytes (JSON)]
//
// Both peers know the expected event type at every read site, so the tag
// is validated on Read to fail fast (and clearly) if the streams desync
// or peers disagree on protocol version.
type EventType byte

const (
	EventTypeTunnelRequest EventType = 1
	EventTypeTunnelCreated EventType = 2
	EventTypeNewConnection EventType = 3
)

// maxFrameSize bounds an individual event payload to protect against
// malicious or buggy peers sending huge length prefixes.
const maxFrameSize = 64 * 1024

type Event[Type TunnelCreated | TunnelRequest | NewConnection] struct {
	Type EventType
	Data *Type
}

func NewTunnelRequestEvent() *Event[TunnelRequest] {
	return &Event[TunnelRequest]{
		Type: EventTypeTunnelRequest,
		Data: &TunnelRequest{},
	}
}

func NewTunnelCreatedEvent() *Event[TunnelCreated] {
	return &Event[TunnelCreated]{
		Type: EventTypeTunnelCreated,
		Data: &TunnelCreated{},
	}
}

func NewConnectionEvent() *Event[NewConnection] {
	return &Event[NewConnection]{
		Type: EventTypeNewConnection,
		Data: &NewConnection{},
	}
}

type TunnelRequest struct {
	Protocol  string
	AuthToken string
}

type TunnelCreated struct {
	Hostname            string
	PublicListenerPort  string
	PrivateListenerPort string
	ErrorMessage        string
}

type NewConnection struct {
	ClientAddr string
}

func (e *Event[Type]) Read(conn io.Reader) error {
	header := make([]byte, 3)
	if _, err := io.ReadFull(conn, header); err != nil {
		return err
	}
	gotType := EventType(header[0])
	if gotType != e.Type {
		return fmt.Errorf("unexpected event type: got %d, want %d", gotType, e.Type)
	}
	length := binary.LittleEndian.Uint16(header[1:])
	if int(length) > maxFrameSize {
		return fmt.Errorf("event frame too large: %d bytes", length)
	}
	buffer := make([]byte, length)
	if _, err := io.ReadFull(conn, buffer); err != nil {
		return err
	}
	return json.Unmarshal(buffer, e.Data)
}

func (e *Event[Type]) Write(conn io.Writer) error {
	data, err := json.Marshal(e.Data)
	if err != nil {
		return err
	}
	if len(data) > maxFrameSize {
		return fmt.Errorf("event payload too large: %d bytes", len(data))
	}
	// Emit the type tag, length prefix, and payload in a single Write so
	// concurrent writers on the same net.Conn cannot interleave frames.
	frame := make([]byte, 3+len(data))
	frame[0] = byte(e.Type)
	binary.LittleEndian.PutUint16(frame[1:3], uint16(len(data)))
	copy(frame[3:], data)
	_, err = conn.Write(frame)
	return err
}

func Bind(src net.Conn, dst net.Conn) error {
	_, err := io.Copy(dst, src)
	// Half-close so the peer can finish draining the other direction
	// (the reverse Bind goroutine).
	if cw, ok := dst.(interface{ CloseWrite() error }); ok {
		_ = cw.CloseWrite()
	} else {
		_ = dst.Close()
	}
	return err
}

func WriteError(eventWriter io.Writer, message string, args ...string) error {
	fmtArgs := make([]any, len(args))
	for i, a := range args {
		fmtArgs[i] = a
	}
	event := Event[TunnelCreated]{
		Type: EventTypeTunnelCreated,
		Data: &TunnelCreated{
			ErrorMessage: fmt.Sprintf(message, fmtArgs...),
		},
	}
	event.Write(eventWriter)
	return errors.New(event.Data.ErrorMessage)
}
