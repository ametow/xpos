package tunnel

import (
	"fmt"
	"net"
	"sync"
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
			connections:   sync.Map{},
			initialBuffer: sync.Map{},
			publicAddr:    hostname,
		},
	}
}

func (tn *HttpTunnel) Init() error {
	privateListener, err := net.Listen("tcp4", ":0")
	if err != nil {
		return err
	}

	tn.privateAddr = fmt.Sprintf("%s:%d", tn.hostname, privateListener.Addr().(*net.TCPAddr).Port)
	go processListener(privateListener, tn.privConnHandler)

	return nil
}

func (tn *HttpTunnel) Close() {
	tn.connections.Range(func(key, value any) bool {
		value.(net.Conn).Close()
		return true
	})
}

func (tn *HttpTunnel) PublicConnHandler(conn net.Conn, buf []byte) {
	tn.initialBuffer.Store(conn.RemoteAddr().String(), buf)
	tn.publicConnHandler(conn)
}
