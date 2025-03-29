package cmd

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"net/netip"

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
		LocalAddr = "127.0.0.1:" + args[0]

		conn, err := net.Dial("tcp", BASEURL)
		if err != nil {
			log.Fatal(err)
		}
		defer conn.Close()

		request := events.NewTunnelRequestEvent()
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
		fmt.Printf("Public addr: http://%s:%s\n", tunnedCreated.Data.Hostname, tunnedCreated.Data.PublicListenerPort)
		fmt.Printf("Private addr: http://%s:%s\n", tunnedCreated.Data.Hostname, tunnedCreated.Data.PrivateListenerPort)
		PrivateAddr = net.JoinHostPort(tunnedCreated.Data.Hostname, tunnedCreated.Data.PrivateListenerPort)

		for {
			newConnectionEvent := events.NewConnectionEvent()
			err := newConnectionEvent.Read(conn)
			if err != nil {
				log.Fatal("error on new connection receive: ", err)
			}

			go func() {
				err := handleConn(newConnectionEvent)
				if err != nil {
					log.Println(err)
				}
			}()
		}
	},
}

func handleConn(client *events.Event[events.NewConnection]) error {
	localConn, err := net.Dial("tcp", LocalAddr)
	if err != nil {
		return err
	}
	defer localConn.Close()
	remoteConn, err := net.Dial("tcp", PrivateAddr)
	if err != nil {
		return err
	}
	defer remoteConn.Close()

	addr, err := netip.ParseAddrPort(client.Data.ClientAddr)
	if err != nil {
		return err
	}

	ip := addr.Addr().As4()
	port := addr.Port()
	buf := make([]byte, 6) // 4 for ip, 2 for port

	copy(buf, ip[:])
	binary.LittleEndian.PutUint16(buf[4:], uint16(port))

	_, err = remoteConn.Write(buf)
	if err != nil {
		return err
	}

	go events.Bind(localConn, remoteConn)
	events.Bind(remoteConn, localConn)
	return nil
}
