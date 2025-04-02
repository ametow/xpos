package events

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

type Event[Type TunnelCreated | TunnelRequest | NewConnection] struct {
	Data *Type
}

func NewTunnelRequestEvent() *Event[TunnelRequest] {
	return &Event[TunnelRequest]{
		Data: &TunnelRequest{},
	}
}

func NewTunnelCreatedEvent() *Event[TunnelCreated] {
	return &Event[TunnelCreated]{
		Data: &TunnelCreated{},
	}
}

func NewConnectionEvent() *Event[NewConnection] {
	return &Event[NewConnection]{
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
	buffer := make([]byte, 2)
	if _, err := conn.Read(buffer); err != nil {
		return err
	}
	length := binary.LittleEndian.Uint16(buffer)
	buffer = make([]byte, length)
	if _, err := conn.Read(buffer); err != nil {
		return err
	}
	err := e.decode(buffer)
	return err
}

func (e *Event[Type]) Write(conn io.Writer) error {
	data, err := e.encode()
	if err != nil {
		return err
	}
	length := make([]byte, 2)
	binary.LittleEndian.PutUint16(length, uint16(len(data)))
	if _, err := conn.Write(length); err != nil {
		return err
	}
	_, err = conn.Write(data)
	return err
}

func (e *Event[Type]) encode() ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(e.Data); err != nil {
		return nil, err
	}
	data := buf.Bytes()
	return data, nil
}

func (e *Event[Type]) decode(data []byte) error {
	buff := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buff)
	return dec.Decode(&e.Data)
}

func Bind(src net.Conn, dst net.Conn) error {
	defer src.Close()
	defer dst.Close()

	buf := make([]byte, 4096)
	for {
		_ = src.SetReadDeadline(time.Now().Add(time.Second * 10))
		n, err := src.Read(buf)
		if err == io.EOF {
			break
		}
		_ = dst.SetWriteDeadline(time.Now().Add(time.Second * 10))
		n, err = dst.Write(buf[:n])
		if err != nil {
			return err
		}
		time.Sleep(10 * time.Millisecond) // rate limit connection bind
	}
	return nil
}

func WriteError(eventWriter io.Writer, message string, args ...string) error {
	event := Event[TunnelCreated]{
		Data: &TunnelCreated{
			ErrorMessage: fmt.Sprintf(message, args),
		},
	}
	event.Write(eventWriter)
	return errors.New(event.Data.ErrorMessage)
}
