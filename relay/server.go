package main

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"

	"github.com/ametow/xpos/events"
	"github.com/ametow/xpos/relay/server"
	"github.com/ametow/xpos/relay/tunnel"
)

type Xpos struct {
	hostname          string
	eventListener     *server.TcpServer
	publicHttpGateway *server.TcpServer
	httpTunnels       *sync.Map
}

func NewXpos() *Xpos {
	x := &Xpos{
		hostname:          "locahost",
		eventListener:     server.New(9876, "event_listener_service"),
		publicHttpGateway: server.New(8080, "public_http_gateway"),
		httpTunnels:       &sync.Map{},
	}
	return x
}

func (x *Xpos) Init() error {
	if err := x.eventListener.Init(); err != nil {
		return err
	}

	if err := x.publicHttpGateway.Init(); err != nil {
		return err
	}

	return nil
}

func (x *Xpos) Start() {
	go x.eventListener.Start(x.serveEvents)
	go x.publicHttpGateway.Start(x.handleHttpGtwConnections)
}

func (x *Xpos) Close() {
	x.eventListener.Close()
	x.publicHttpGateway.Close()
}

func (x *Xpos) serveEvents(conn net.Conn) error {
	defer conn.Close()

	req := events.NewTunnelRequestEvent()
	err := req.Read(conn)
	if err != nil {
		return err
	}

	var user, hostname string

	// TODO(set user here)
	user = "arslan"

	hostname = user + "." + x.hostname

	var tn tunnel.Tunnel
	switch req.Data.Protocol {
	case "http":
		_, ok := x.httpTunnels.Load(hostname)
		if ok {
			return errors.New("host is busy")
		}
		tn = tunnel.NewHttpTunnel(hostname, conn)
		x.httpTunnels.Store(hostname, tn)
	case "tcp":
		tn = tunnel.NewTcpTunnel(conn)
	default:
		return nil
	}

	tn.Init()
	defer tn.Close()

	tunnelCreatedEvent := events.NewTunnelCreatedEvent()
	tunnelCreatedEvent.Data.Hostname = x.hostname
	tunnelCreatedEvent.Data.PublicListenerPort = tn.PublicAddr()
	tunnelCreatedEvent.Data.PrivateListenerPort = tn.PrivateAddr()

	err = tunnelCreatedEvent.Write(conn)
	if err != nil {
		return err
	}

	buf := make([]byte, 8)
	for {
		_, err := conn.Read(buf)
		if err != nil {
			return errors.New("agent disconnected")
		}
	}

}

func (x *Xpos) handleHttpGtwConnections(con net.Conn) error {
	reader := bufio.NewReader(con)

	firstLine, isPrefix, err := reader.ReadLine()
	if err != nil || isPrefix {
		return fmt.Errorf("failed to read request line: %v", err)
	}

	line := string(firstLine)
	if !strings.HasPrefix(line, "Host:") {
		return fmt.Errorf("failed to extract host: %v", err)
	}

	host := strings.TrimSpace(line[len("Host:"):])
	log.Println("request for host: ", host)

	tn, ok := x.httpTunnels.Load(host)
	if !ok {
		return errors.New("no tunnel created for this host")
	}
	httpTunnel, ok := tn.(*tunnel.HttpTunnel)
	if !ok {
		return errors.New("tunnel is closed")
	}

	httpTunnel.PublicConnHandler(con)
	return nil
}
