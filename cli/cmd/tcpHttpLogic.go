package cmd

import (
	"fmt"
	"log"
	"net"

	"github.com/ametow/xpos/cli/handler"
	"github.com/ametow/xpos/events"
)

func tcpHttpCommand(protocol, port string) {
	conn, err := net.Dial("tcp4", BASEURL)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	request := events.NewTunnelRequestEvent()
	request.Data.Protocol = protocol
	err = request.Write(conn)
	if err != nil {
		log.Fatal("error requesting tunnel:", err)
	}

	tunnedCreated := events.NewTunnelCreatedEvent()
	err = tunnedCreated.Read(conn)
	if err != nil {
		log.Fatal("error creating tunnel:", err)
	}

	fmt.Println("Started listening on public network.")
	fmt.Printf("Public addr: %s://%s\n", protocol, tunnedCreated.Data.PublicListenerPort)

	privateAddr := tunnedCreated.Data.PrivateListenerPort
	localAddr := net.JoinHostPort("127.0.0.1", port)

	for {
		newConnectionEvent := events.NewConnectionEvent()
		err := newConnectionEvent.Read(conn)
		if err != nil {
			log.Fatal("error on new connection receive: ", err)
		}

		go func() {
			err := handler.HandleConn(newConnectionEvent, localAddr, privateAddr)
			if err != nil {
				log.Println(err)
			}
		}()
	}

}
