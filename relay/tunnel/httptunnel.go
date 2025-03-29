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
	privLn, err := net.Listen("tcp", "0.0.0.0:")
	if err != nil {
		log.Println(err)
		return
	}
	tn.privateAddr = privLn.Addr().String()
	go processListener(privLn, tn.privConnHandler)
}

func (tn *HttpTunnel) Close() {
	for port, cn := range tn.connections {
		cn.Close()
		delete(tn.connections, port)
		delete(tn.initialBuffer, port)
	}
}

func (tn *HttpTunnel) PublicConnHandler(conn net.Conn) {
	tn.publicConnHandler(conn)
}
