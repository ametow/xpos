package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"

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
		hostname:          "localhost",
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
	go func() {
		err := x.eventListener.Start(x.serveEvents)
		if err != nil {
			log.Println(err)
		}
	}()
	go func() {
		err := x.publicHttpGateway.Start(x.handleHttpGtwConnections)
		if err != nil {
			log.Println(err)
		}
	}()
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

	hostname = user + "." + x.hostname + ":8080"

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

	fmt.Println(x.httpTunnels)

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
	con.SetReadDeadline(time.Now().Add(3 * time.Second))
	host, buffer, err := parseHost(con)
	if err != nil || host == "" {
		return err
	}

	host = strings.ToLower(host)

	tn, ok := x.httpTunnels.Load(host)
	if !ok {
		return errors.New("no tunnel created for this host")
	}
	httpTunnel, ok := tn.(*tunnel.HttpTunnel)
	if !ok {
		return errors.New("tunnel is closed")
	}

	httpTunnel.PublicConnHandler(con, buffer)
	return nil
}

func parseHost(r io.Reader) (string, []byte, error) {
	buffer := make([]byte, 2048)
	size, err := r.Read(buffer)
	buffer = buffer[:size]
	if err != nil {
		return "", buffer, err
	}
	text := string(buffer)
	left := strings.Index(text, "Host: ")
	if left < 0 {
		left = strings.Index(text, "host: ")
	}
	if left < 0 {
		return "", buffer, fmt.Errorf("no host detected")
	}
	text = text[left+6:] // drops chars "Host: "
	right := strings.Index(text, "\n")
	if right < 0 {
		return "", buffer, fmt.Errorf("no host detected")
	}
	return strings.TrimSpace(text[:right]), buffer, nil
}
