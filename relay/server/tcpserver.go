package server

import (
	"fmt"
	"log"
	"net"
)

type TcpServer struct {
	name string
	ln   net.Listener
	port uint16
}

func New(port uint16, name string) *TcpServer {
	return &TcpServer{port: port, name: name}
}

func (s TcpServer) Close() {
	s.ln.Close()
}

func (s *TcpServer) Init() error {
	ln, err := net.Listen("tcp4", fmt.Sprintf("%s:%d", "localhost", s.port))
	if err != nil {
		return err
	}
	s.ln = ln
	return nil
}

func (s *TcpServer) Start(handler func(net.Conn) error) error {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return err
		}
		go func() {
			err := handler(conn)
			if err != nil {
				log.Printf("[%s]: %s\n", s.name, err.Error())
			}
		}()
	}
}
