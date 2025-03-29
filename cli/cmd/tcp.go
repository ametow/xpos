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
	rootCmd.AddCommand(tcpCommand)
}

var tcpCommand = &cobra.Command{
	Use:   "tcp [port]",
	Short: "Forward tcp traffic",
	Long:  ``,
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		localAddr := net.JoinHostPort("127.0.0.1", args[0])

		conn, err := net.Dial("tcp4", BASEURL)
		if err != nil {
			log.Fatal(err)
		}
		defer conn.Close()

		request := events.NewTunnelRequestEvent()
		request.Data.Protocol = "tcp"
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
		fmt.Printf("Public addr: %s\n", tunnedCreated.Data.PublicListenerPort)
		fmt.Printf("Private addr: %s\n", tunnedCreated.Data.PrivateListenerPort)
		// privateAddr = net.JoinHostPort(tunnedCreated.Data.Hostname, tunnedCreated.Data.PrivateListenerPort)
		privateAddr := tunnedCreated.Data.PrivateListenerPort

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
	},
}
