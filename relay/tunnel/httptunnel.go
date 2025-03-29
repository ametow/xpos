package tunnel

import (
	"log"
	"net"
)

type HttpTunnel struct {
	TcpTunnel
	hostname string
}

func NewHttpTunnel(hostname string, conn net.Conn) Tunnel {
	return &HttpTunnel{
		hostname: hostname,
		TcpTunnel: TcpTunnel{
			AgentConn:     conn,
			connections:   make(map[string]net.Conn),
			initialBuffer: make(map[string][]byte),
		},
	}
}

func (tn *HttpTunnel) PrivateAddr() string {
	return tn.privateAddr
}

func (tn *HttpTunnel) PublicAddr() string {
	return tn.hostname
}

func (tn *HttpTunnel) Init() {
	privateListener, err := net.Listen("tcp4", "localhost:")
	if err != nil {
		log.Println(err)
		return
	}
	tn.privateAddr = privateListener.Addr().String()
	go processListener(privateListener, tn.privConnHandler)
}

func (tn *HttpTunnel) Close() {
	for port, cn := range tn.connections {
		cn.Close()
		delete(tn.connections, port)
		delete(tn.initialBuffer, port)
	}
}

func (tn *HttpTunnel) PublicConnHandler(conn net.Conn, buf []byte) {
	tn.initialBuffer[conn.RemoteAddr().String()] = buf
	tn.publicConnHandler(conn)
}
