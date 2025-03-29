package cmd

import (
	"fmt"
	"log"
	"net"

	"github.com/ametow/xpos/events"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(httpCommand)
}

var httpCommand = &cobra.Command{
	Use:   "http [port]",
	Short: "Forward http traffic",
	Long:  ``,
	Args:  cobra.MinimumNArgs(1),
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
