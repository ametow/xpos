package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"net"

	"github.com/ametow/xpos/events"
)

const BASEURL = "localhost:4321"

var PrivateAddr string
var LocalAddr string = "localhost:7777"

func main() {
	flag.Parse()

	conn, err := net.Dial("tcp", BASEURL)
	if err != nil {
		panic(err)
	}

	request := &events.Event[events.TunnelRequest]{
		Data: &events.TunnelRequest{},
	}
	err = request.Write(conn)
	if err != nil {
		log.Fatal("error requesting tunnel:", err)
	}

	tunnedCreated := &events.Event[events.TunnelCreated]{
		Data: &events.TunnelCreated{},
	}
	err = tunnedCreated.Read(conn)
	if err != nil {
		log.Fatal("error creating tunnel:", err)
	}

	fmt.Println("Started listening on public network.")
	fmt.Println("Public addr: ", tunnedCreated.Data.PublicAddr)
	fmt.Println("Private addr: ", tunnedCreated.Data.PrivateAddr)
	PrivateAddr = tunnedCreated.Data.PrivateAddr

	for {
		newconn := &events.Event[events.NewConnection]{Data: &events.NewConnection{}}
		err := newconn.Read(conn)
		if err != nil {
			log.Fatal("error on new connection receive: ", err)
		}

		go handleConn(newconn)
	}
}

func handleConn(client *events.Event[events.NewConnection]) {
	// local dial
	localConn, err := net.Dial("tcp", LocalAddr)
	if err != nil {
		log.Fatal(err)
	}
	// remote dial
	remoteConn, err := net.Dial("tcp", PrivateAddr)
	if err != nil {
		log.Fatal(err)
	}

	buf := make([]byte, 2)
	binary.LittleEndian.PutUint16(buf, uint16(client.Data.ClientPort))

	_, err = remoteConn.Write(buf)
	if err != nil {
		log.Fatal(err)
	}

	go events.Bind(localConn, remoteConn)
	events.Bind(remoteConn, localConn)
}
