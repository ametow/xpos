package tunnel

import (
	"log"
	"net"
)

type Tunnel interface {
	PrivateAddr() string
	PublicAddr() string
	Init()
	Close()
}

func processListener(ln net.Listener, handler func(net.Conn)) {
	defer ln.Close()
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println(err)
			return
		}
		go handler(conn)
	}
}
