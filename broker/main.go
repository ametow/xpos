package main

import (
	"fmt"
	"log"
	"net"

	"github.com/ametow/xpos/broker/tunnel"
	"github.com/ametow/xpos/events"
)

func main() {
	ln, err := net.Listen("tcp4", "0.0.0.0:4321")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Started listening on :4321")

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Fatal(err)
		}

		go processConn(conn)
	}
}

func processConn(conn net.Conn) {
	defer conn.Close()

	req := &events.Event[events.TunnelRequest]{Data: &events.TunnelRequest{}}

	err := req.Read(conn)
	if err != nil {
		fmt.Println(err)
		return
	}

	tunnel := tunnel.New(conn)
	tunnel.Init()
	defer tunnel.Close()

	tunnelCreatedEvent := &events.Event[events.TunnelCreated]{
		Data: &events.TunnelCreated{
			Hostname:            "34.229.0.117",
			PublicListenerPort:  tunnel.PublicLn(),
			PrivateListenerPort: tunnel.PrivateLn(),
		},
	}

	err = tunnelCreatedEvent.Write(conn)
	if err != nil {
		fmt.Println(err)
		return
	}

	buf := make([]byte, 8)
	for {
		_, err := conn.Read(buf)
		if err != nil {
			fmt.Println(err)
			return
		}
	}
}
