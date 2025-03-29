package handler

import (
	"encoding/binary"
	"net"
	"net/netip"

	"github.com/ametow/xpos/events"
)

func HandleConn(client *events.Event[events.NewConnection], local, private string) error {
	localConn, err := net.Dial("tcp4", local)
	if err != nil {
		return err
	}
	defer localConn.Close()
	remoteConn, err := net.Dial("tcp4", private)
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
