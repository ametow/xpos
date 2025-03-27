package tunnel

import (
	"encoding/binary"
	"log"
	"net"
	"strconv"
	"sync"

	"github.com/ametow/xpos/events"
)

type Tunnel struct {
	agentConn   net.Conn
	connections map[uint16]net.Conn
	mutex       sync.RWMutex
	privateAddr string
	publicAddr  string
}

func New(conn net.Conn) *Tunnel {
	return &Tunnel{
		agentConn:   conn,
		connections: make(map[uint16]net.Conn),
	}
}

func (tn *Tunnel) PrivateLn() string {
	return tn.privateAddr
}

func (tn *Tunnel) PublicLn() string {
	return tn.publicAddr
}

func (tn *Tunnel) Init() {
	pubLn, err := net.Listen("tcp", "0.0.0.0:")
	if err != nil {
		log.Fatal(err)
	}
	privLn, err := net.Listen("tcp", "0.0.0.0:")
	if err != nil {
		log.Fatal(err)
	}
	tn.privateAddr = strconv.Itoa(privLn.Addr().(*net.TCPAddr).Port)
	tn.publicAddr = strconv.Itoa(pubLn.Addr().(*net.TCPAddr).Port)

	go processListener(privLn, tn.privConnHandler)
	go processListener(pubLn, tn.publicConnHandler)
}

func processListener(ln net.Listener, handler func(net.Conn)) {
	defer ln.Close()
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go handler(conn)
	}
}

func (tn *Tunnel) publicConnHandler(conn net.Conn) {
	ip := conn.RemoteAddr().(*net.TCPAddr).IP
	port := uint16(conn.RemoteAddr().(*net.TCPAddr).Port)

	newConnEvent := &events.Event[events.NewConnection]{
		Data: &events.NewConnection{
			ClientIP:   ip,
			ClientPort: port,
		},
	}

	err := newConnEvent.Write(tn.agentConn)
	if err != nil {
		conn.Close()
		log.Fatal(err)
	}
	tn.connections[port] = conn
}

func (tn *Tunnel) privConnHandler(conn net.Conn) {
	defer conn.Close()

	buf := make([]byte, 2)
	_, err := conn.Read(buf)
	if err != nil {
		log.Fatal(err)
	}

	port := binary.LittleEndian.Uint16(buf)
	clientConn, exists := tn.connections[port]
	if !exists {
		log.Fatal("client conn not found")
	}
	defer clientConn.Close()
	delete(tn.connections, port)

	go events.Bind(conn, clientConn)
	events.Bind(clientConn, conn)
}

func (tn *Tunnel) Close() {
	// clean up resources
}
