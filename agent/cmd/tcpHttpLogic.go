package cmd

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/ametow/xpos/agent/config"
	"github.com/ametow/xpos/agent/handler"
	"github.com/ametow/xpos/events"
)

func tcpHttpCommand(protocol, port string, debug bool) {
	var conf config.Config
	if err := conf.Load(); err != nil {
		fmt.Println(err)
		return
	}

	conn, err := net.Dial("tcp4", conf.Remote.Events)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	request := events.NewTunnelRequestEvent()
	request.Data.Protocol = protocol
	request.Data.AuthToken = conf.Local.AuthToken

	err = request.Write(conn)
	if err != nil {
		fmt.Println("error requesting tunnel:", err)
		return
	}

	tunnelCreated := events.NewTunnelCreatedEvent()
	err = tunnelCreated.Read(conn)
	if err != nil {
		fmt.Println("error creating tunnel:", err)
		return
	}
	if tunnelCreated.Data.ErrorMessage != "" {
		fmt.Println(tunnelCreated.Data.ErrorMessage)
		return
	}

	if protocol == "http" {
		protocol = "https"
	}

	if debug && protocol == "https" {
		go startDebugProxy(net.JoinHostPort("127.0.0.1", port))
	}

	localAddr := net.JoinHostPort("127.0.0.1", port)

	fmt.Println("Started listening on public network.")
	fmt.Printf("Protocol: \t %s \n", strings.ToUpper(request.Data.Protocol))
	fmt.Printf("Forwarded: \t %s://%s -> %s \n", protocol, tunnelCreated.Data.PublicListenerPort, localAddr)

	for {
		newConnectionEvent := events.NewConnectionEvent()
		err := newConnectionEvent.Read(conn)
		if err != nil {
			log.Fatal("error on new connection receive: ", err)
		}

		go func() {
			err := handler.HandleConn(newConnectionEvent, localAddr, tunnelCreated.Data.PrivateListenerPort)
			if err != nil {
				log.Println(err)
			}
		}()
	}

}

func startDebugProxy(localAddr string) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Printf("failed to start debug proxy: %v", err)
		return
	}
	port := ln.Addr().(*net.TCPAddr).Port
	fmt.Printf("Debug proxy: http://127.0.0.1:%d\n", port)

	proxy := httputil.NewSingleHostReverseProxy(&url.URL{Scheme: "http", Host: localAddr})
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Printf("[DEBUG] %s %s\n", r.Method, r.URL.Path)
			proxy.ServeHTTP(w, r)
		}),
	}
	if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
		log.Printf("debug proxy stopped: %v", err)
	}
}
