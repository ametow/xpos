package cmd

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"

	"github.com/ametow/xpos/events"
	"github.com/spf13/cobra"
)

const BASEURL = "34.229.0.117:4321"

func init() {
	rootCmd.AddCommand(tcpCommand)
}

var tcpCommand = &cobra.Command{
	Use:   "tcp [port]",
	Short: "Forward tcp traffic",
	Long:  `All software has versions.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		LocalAddr = "127.0.0.1:" + args[0]

		conn, err := net.Dial("tcp", BASEURL)
		if err != nil {
			log.Fatal(err)
		}
		defer conn.Close()

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
		fmt.Printf("Public addr: http://%s:%s\n", tunnedCreated.Data.Hostname, tunnedCreated.Data.PublicListenerPort)
		fmt.Printf("Private addr: http://%s:%s\n", tunnedCreated.Data.Hostname, tunnedCreated.Data.PrivateListenerPort)
		PrivateAddr = net.JoinHostPort(tunnedCreated.Data.Hostname, tunnedCreated.Data.PrivateListenerPort)

		for {
			newConnectionEvent := &events.Event[events.NewConnection]{Data: &events.NewConnection{}}
			err := newConnectionEvent.Read(conn)
			if err != nil {
				log.Fatal("error on new connection receive: ", err)
			}

			go handleConn(newConnectionEvent)
		}
	},
}

var PrivateAddr string
var LocalAddr string

func handleConn(client *events.Event[events.NewConnection]) {
	// local dial
	localConn, err := net.Dial("tcp", LocalAddr)
	if err != nil {
		log.Fatal(err)
	}
	// remote dial
	remoteConn, err := net.Dial("tcp4", PrivateAddr)
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
