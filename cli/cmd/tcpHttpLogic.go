package cmd

import (
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/ametow/xpos/cli/config"
	"github.com/ametow/xpos/cli/handler"
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

	err = request.Write(conn)
	if err != nil {
		fmt.Println("error requesting tunnel:", err)
		return
	}

	tunnelCreated := events.NewTunnelCreatedEvent()
	err = tunnelCreated.Read(conn)
	if err != nil {
		fmt.Println("error creating tunnel:", err)
		return
	}
	if tunnelCreated.Data.ErrorMessage != "" {
		fmt.Println(tunnelCreated.Data.ErrorMessage)
		return
	}

	if protocol == "http" {
		protocol = "https"
	}
	localAddr := net.JoinHostPort("127.0.0.1", port)

	fmt.Println("Started listening on public network.")
	fmt.Printf("Protocol: \t %s \n", strings.ToUpper(request.Data.Protocol))
	fmt.Printf("Forwarded: \t %s://%s -> %s \n", protocol, tunnelCreated.Data.PublicListenerPort, localAddr)

	for {
		newConnectionEvent := events.NewConnectionEvent()
		err := newConnectionEvent.Read(conn)
		if err != nil {
			log.Fatal("error on new connection receive: ", err)
		}

		go func() {
			err := handler.HandleConn(newConnectionEvent, localAddr, tunnelCreated.Data.PrivateListenerPort)
			if err != nil {
				log.Println(err)
			}
		}()
	}

}
