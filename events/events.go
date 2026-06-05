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
//	[1 byte protocol version][1 byte type tag][2 bytes little-endian payload length][payload bytes (JSON)]
//
// Both peers know the expected event type at every read site, so the tag
// is validated on Read to fail fast (and clearly) if the streams desync
// or peers disagree on protocol version. The version byte lets us
// evolve the framing format without silently corrupting older agents.
type EventType byte

const (
	EventTypeTunnelRequest EventType = 1
	EventTypeTunnelCreated EventType = 2
	// EventTypeOpenStream is the first frame sent on each yamux
	// stream the relay opens for a new public connection. It tells
	// the agent which public client this stream belongs to and
	// carries any buffered initial bytes (used for HTTP, where the
	// relay has already consumed the Host: header before opening
	// the stream).
	EventTypeOpenStream EventType = 4
)

// ProtocolVersion is the current wire-format version. Peers that read
// a different value should refuse the connection.
//
// v2 (this version): NewConnection is gone; per-public-conn streams
// are multiplexed via yamux over the agent control connection,
// and the first frame on each stream is an OpenStream event.
//
// v1: separate "private" TCP listener per tunnel; agent dialed back
// per public conn and identified the conn by ip:port bytes.
const ProtocolVersion byte = 2

// maxFrameSize bounds an individual event payload to protect against
// malicious or buggy peers sending huge length prefixes.
const maxFrameSize = 64 * 1024

type Event[Type TunnelCreated | TunnelRequest | OpenStream] struct {
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

// NewOpenStreamEvent returns an Event[OpenStream] ready to be written
// to (or read from) the first frame of a yamux stream.
func NewOpenStreamEvent() *Event[OpenStream] {
	return &Event[OpenStream]{
		Type: EventTypeOpenStream,
		Data: &OpenStream{},
	}
}

type TunnelRequest struct {
	Protocol  string
	AuthToken string
}

type TunnelCreated struct {
	Hostname           string
	PublicListenerPort string
	// PrivateListenerPort is retained on the wire for backwards
	// inspection but is always empty in v2 — the agent no longer
	// dials a private port; it accepts yamux streams instead.
	PrivateListenerPort string
	ErrorMessage        string
}

// OpenStream is the first frame on each yamux stream the relay opens
// for a new public connection.
type OpenStream struct {
	// ClientAddr is the public client's remote address as observed
	// by the relay. Used by the agent for logging only.
	ClientAddr string
	// InitialData carries any bytes the relay has already read from
	// the public connection before opening the stream. For HTTP
	// tunnels this is the partial request the relay parsed to find
	// the Host header; for plain TCP it is empty.
	InitialData []byte
}

func (e *Event[Type]) Read(conn io.Reader) error {
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return err
	}
	if header[0] != ProtocolVersion {
		return fmt.Errorf("unsupported protocol version: got %d, want %d", header[0], ProtocolVersion)
	}
	gotType := EventType(header[1])
	if gotType != e.Type {
		return fmt.Errorf("unexpected event type: got %d, want %d", gotType, e.Type)
	}
	length := binary.LittleEndian.Uint16(header[2:])
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
	// Emit version, type tag, length prefix, and payload in a single
	// Write so concurrent writers on the same net.Conn cannot
	// interleave frames.
	frame := make([]byte, 4+len(data))
	frame[0] = ProtocolVersion
	frame[1] = byte(e.Type)
	binary.LittleEndian.PutUint16(frame[2:4], uint16(len(data)))
	copy(frame[4:], data)
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
