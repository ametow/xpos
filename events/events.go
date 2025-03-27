package events

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"io"
	"net"
	"time"
)

type Event[Type TunnelCreated | TunnelRequest | NewConnection] struct {
	Data *Type
}

type TunnelRequest struct {
}

type TunnelCreated struct {
	Hostname            string
	PublicListenerPort  string
	PrivateListenerPort string
}

type NewConnection struct {
	ClientIP   net.IP
	ClientPort uint16
}

func (e *Event[Type]) Read(conn net.Conn) error {
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

func (e *Event[Type]) Write(conn net.Conn) error {
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
		_, err = dst.Write(buf[:n])
		if err != nil {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil
}
