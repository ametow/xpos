package cmd

import (
	"fmt"
	"io"
	"log"
	"net"
	"strings"

	"github.com/hashicorp/yamux"

	"github.com/ametow/xpos/agent/config"
	"github.com/ametow/xpos/agent/handler"
	"github.com/ametow/xpos/events"
)

func tcpHttpCommand(protocol, port string) {
	var conf config.Config
	if err := conf.Load(); err != nil {
		fmt.Println(err)
		return
	}

	conn, err := net.Dial("tcp4", conf.Remote.Events)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	request := events.NewTunnelRequestEvent()
	request.Data.Protocol = protocol
	request.Data.AuthToken = conf.Local.AuthToken

	if err := request.Write(conn); err != nil {
		fmt.Println("error requesting tunnel:", err)
		return
	}

	tunnelCreated := events.NewTunnelCreatedEvent()
	if err := tunnelCreated.Read(conn); err != nil {
		fmt.Println("error creating tunnel:", err)
		return
	}
	if tunnelCreated.Data.ErrorMessage != "" {
		fmt.Println(tunnelCreated.Data.ErrorMessage)
		return
	}

	displayProto := protocol
	if displayProto == "http" {
		displayProto = "https"
	}
	localAddr := net.JoinHostPort("127.0.0.1", port)

	fmt.Println("Started listening on public network.")
	fmt.Printf("Protocol: \t %s \n", strings.ToUpper(request.Data.Protocol))
	fmt.Printf("Forwarded: \t %s://%s -> %s \n", displayProto, tunnelCreated.Data.PublicListenerPort, localAddr)

	// Wrap the control connection in a yamux client. From here on
	// the relay multiplexes public connections as yamux streams;
	// the agent just accepts and bridges each one.
	cfg := yamux.DefaultConfig()
	cfg.EnableKeepAlive = false
	cfg.LogOutput = io.Discard
	session, err := yamux.Client(conn, cfg)
	if err != nil {
		log.Fatal("yamux client:", err)
	}
	defer session.Close()

	if err := handler.ServeStreams(session, localAddr); err != nil {
		log.Println("serve streams:", err)
	}
}
