package tunnel

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/ametow/xpos/events"
)

type TcpTunnel struct {
	AgentConn     net.Conn
	connections   sync.Map
	privateAddr   string
	publicAddr    string
	initialBuffer sync.Map
}

func NewTcpTunnel(conn net.Conn) Tunnel {
	return &TcpTunnel{
		AgentConn:     conn,
		connections:   sync.Map{},
		initialBuffer: sync.Map{},
	}
}

func (tn *TcpTunnel) PrivateAddr() string {
	return tn.privateAddr
}

func (tn *TcpTunnel) PublicAddr() string {
	return tn.publicAddr
}

func (tn *TcpTunnel) Init() error {
	pubLn, err := net.Listen("tcp4", "localhost:")
	if err != nil {
		return err
	}
	privLn, err := net.Listen("tcp4", "localhost:")
	if err != nil {
		return err
	}
	tn.privateAddr = privLn.Addr().String()
	tn.publicAddr = pubLn.Addr().String()

	go processListener(privLn, tn.privConnHandler)
	go processListener(pubLn, tn.publicConnHandler)
	return nil
}

func (tn *TcpTunnel) publicConnHandler(conn net.Conn) error {
	clientAddr := conn.RemoteAddr().String()

	newConnEvent := events.NewConnectionEvent()
	newConnEvent.Data.ClientAddr = clientAddr

	err := newConnEvent.Write(tn.AgentConn)
	if err != nil {
		conn.Close()
		return err
	}
	tn.connections.Store(clientAddr, conn)
	return nil
}

func (tn *TcpTunnel) privConnHandler(conn net.Conn) error {
	defer conn.Close()

	buf := make([]byte, 6)
	_, err := conn.Read(buf)
	if err != nil {
		return err
	}

	ip := net.IP(buf[:4])                       // First ip 4 bytes
	port := binary.LittleEndian.Uint16(buf[4:]) // 2 bytes for port
	addr := net.JoinHostPort(ip.String(), fmt.Sprintf("%d", port))

	clientVal, exists := tn.connections.Load(addr)
	if !exists {
		return errors.New("client conn not found")
	}
	clientConn, ok := clientVal.(net.Conn)
	if !ok {
		return errors.New("connection closed")
	}
	defer clientConn.Close()
	tn.connections.Delete(addr)

	defer tn.initialBuffer.Delete(addr)

	initBuf, ok := tn.initialBuffer.Load(addr)

	if ok && len(initBuf.([]byte)) > 0 {
		if _, err := conn.Write(initBuf.([]byte)); err != nil {
			return err
		}
	}

	go events.Bind(conn, clientConn)
	events.Bind(clientConn, conn)
	return nil
}

func (tn *TcpTunnel) Close() {
	tn.connections.Range(func(key, value any) bool {
		value.(net.Conn).Close()
		return true
	})
}
