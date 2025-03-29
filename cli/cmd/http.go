package cmd

import (
	"fmt"
	"log"
	"net"

	"github.com/ametow/xpos/cli/handler"
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
		localAddr := net.JoinHostPort("127.0.0.1", args[0])

		conn, err := net.Dial("tcp4", BASEURL)
		if err != nil {
			log.Fatal(err)
		}
		defer conn.Close()

		request := &events.Event[events.TunnelRequest]{
			Data: &events.TunnelRequest{Protocol: "http"},
		}
		err = request.Write(conn)
		if err != nil {
			log.Fatal("error requesting tunnel:", err)
		}

		tunnelCreated := &events.Event[events.TunnelCreated]{
			Data: &events.TunnelCreated{},
		}
		err = tunnelCreated.Read(conn)
		if err != nil {
			log.Fatal("error creating tunnel:", err)
		}

		fmt.Println("Started listening on public network.")
		fmt.Printf("Public addr: http://%s\n", tunnelCreated.Data.PublicListenerPort)
		fmt.Printf("Private addr: http://%s\n", tunnelCreated.Data.PrivateListenerPort)
		// privateAddr = net.JoinHostPort(tunnedCreated.Data.Hostname, tunnedCreated.Data.PrivateListenerPort)
		privateAddr := tunnelCreated.Data.PrivateListenerPort

		for {
			newConnectionEvent := &events.Event[events.NewConnection]{Data: &events.NewConnection{}}
			err := newConnectionEvent.Read(conn)
			if err != nil {
				log.Fatal("error on new connection receive: ", err)
			}

			go handler.HandleConn(newConnectionEvent, localAddr, privateAddr)
		}
	},
}
