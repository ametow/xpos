package tunnel

import (
	"encoding/binary"
	"log"
	"net"
	"strconv"
	"sync"

	"github.com/ametow/xpos/events"
)

type TcpTunnel struct {
	AgentConn      net.Conn
	connections    map[string]net.Conn
	agentConnMutex sync.Mutex
	connMutex      sync.Mutex
	privateAddr    string
	publicAddr     string
	initialBuffer  map[string][]byte
}

func NewTcpTunnel(conn net.Conn) Tunnel {
	return &TcpTunnel{
		AgentConn:     conn,
		connections:   make(map[string]net.Conn),
		initialBuffer: make(map[string][]byte),
	}
}

func (tn *TcpTunnel) PrivateAddr() string {
	return tn.privateAddr
}

func (tn *TcpTunnel) PublicAddr() string {
	return tn.publicAddr
}

func (tn *TcpTunnel) Init() {
	pubLn, err := net.Listen("tcp", "0.0.0.0:")
	if err != nil {
		log.Println(err)
		return
	}
	privLn, err := net.Listen("tcp", "0.0.0.0:")
	if err != nil {
		log.Println(err)
		return
	}
	tn.privateAddr = privLn.Addr().String()
	tn.publicAddr = pubLn.Addr().String()

	go processListener(privLn, tn.privConnHandler)
	go processListener(pubLn, tn.publicConnHandler)
}

func (tn *TcpTunnel) publicConnHandler(conn net.Conn) {
	clientAddr := conn.RemoteAddr().String()
	newConnEvent := &events.Event[events.NewConnection]{
		Data: &events.NewConnection{
			ClientAddr: clientAddr,
		},
	}

	tn.agentConnMutex.Lock()
	defer tn.agentConnMutex.Unlock()

	err := newConnEvent.Write(tn.AgentConn)
	if err != nil {
		conn.Close()
		log.Println(err)
		return
	}
	tn.connections[clientAddr] = conn
	tn.initialBuffer[clientAddr] = nil
}

func (tn *TcpTunnel) privConnHandler(conn net.Conn) {
	defer conn.Close()

	buf := make([]byte, 6)
	_, err := conn.Read(buf)
	if err != nil {
		log.Println(err)
		return
	}

	ip := net.IP(buf[:4])                       // First ip 4 bytes
	port := binary.LittleEndian.Uint16(buf[4:]) // 2 bytes for port

	addr := net.JoinHostPort(ip.String(), strconv.Itoa(int(port)))

	tn.connMutex.Lock()
	clientConn, exists := tn.connections[addr]
	if !exists {
		log.Println("client conn not found")
		return
	}
	defer clientConn.Close()
	delete(tn.connections, addr)

	tn.connMutex.Unlock()

	defer delete(tn.initialBuffer, addr)

	if len(tn.initialBuffer[addr]) > 0 {
		if _, err := conn.Write(tn.initialBuffer[addr]); err != nil {
			log.Println(err)
			return
		}
	}

	go events.Bind(conn, clientConn)
	events.Bind(clientConn, conn)
}

func (tn *TcpTunnel) Close() {
	for port, cn := range tn.connections {
		cn.Close()
		delete(tn.connections, port)
		delete(tn.initialBuffer, port)
	}
}
