package xpos

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ametow/xpos/events"
	"github.com/ametow/xpos/relay/auth"
	"github.com/ametow/xpos/relay/server"
	"github.com/ametow/xpos/relay/tunnel"
)

type Xpos struct {
	hostname      string
	eventServer   *server.TcpServer
	httpGateway   *server.TcpServer
	httpTunnels   *sync.Map
	authenticator auth.Authenticator
}

func New() *Xpos {
	x := &Xpos{
		hostname:      os.Getenv("XPOS_DOMAIN"),
		eventServer:   server.New(9876, "event_server"),
		httpGateway:   server.New(8080, "http_gateway"),
		httpTunnels:   &sync.Map{},
		authenticator: auth.New(),
	}
	return x
}

func (x *Xpos) Init() error {
	if err := x.eventServer.Init(); err != nil {
		return err
	}

	if err := x.httpGateway.Init(); err != nil {
		return err
	}

	return nil
}

func (x *Xpos) Start() {
	go func() {
		err := x.eventServer.Start(x.handleEventServer)
		if err != nil {
			log.Println(err)
		}
	}()
	go func() {
		err := x.httpGateway.Start(x.handleHttpGateway)
		if err != nil {
			log.Println(err)
		}
	}()
}

func (x *Xpos) Close() {
	x.eventServer.Close()
	x.httpGateway.Close()
}

func (x *Xpos) handleEventServer(conn net.Conn) error {
	defer conn.Close()

	req := events.NewTunnelRequestEvent()
	err := req.Read(conn)
	if err != nil {
		return err
	}

	if req.Data.Protocol != "tcp" && req.Data.Protocol != "http" {
		return events.WriteError(conn, "invalid protocol %s", req.Data.Protocol)
	}

	user, err := x.authenticator.Authenticate(req.Data.AuthToken)
	if err != nil {
		return events.WriteError(conn, "authentication failed %s", "\n\trequest auth token from https://xpos-it.com/auth\n")
	}

	hostname := fmt.Sprintf("%s.%s", user.Login, x.hostname)

	var tn tunnel.Tunnel
	switch req.Data.Protocol {
	case "http":
		_, ok := x.httpTunnels.Load(hostname)
		if ok {
			return events.WriteError(conn, "subdomain is busy: %s, try another one", user.Login)
		}
		tn = tunnel.NewHttpTunnel(hostname, conn)
		x.httpTunnels.Store(hostname, tn)
		defer x.httpTunnels.Delete(hostname)
	case "tcp":
		tn = tunnel.NewTcpTunnel(conn, hostname)
	default:
		return nil
	}

	if err := tn.Init(); err != nil {
		return events.WriteError(conn, "failed to create tunnel", err.Error())
	}
	defer tn.Close()

	fmt.Printf("%s [tunnel-created] %s\n", time.Now().Format("2006/01/02 15:04:05"), user.Login)

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

func (x *Xpos) handleHttpGateway(con net.Conn) error {
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
