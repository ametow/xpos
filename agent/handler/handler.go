package handler

import (
	"fmt"
	"io"
	"log"
	"net"
	"sync"

	"github.com/hashicorp/yamux"

	"github.com/ametow/xpos/events"
)

// ServeStreams accepts yamux streams from the relay (each one
// corresponds to a new public connection) and bridges them to the
// local target address. It blocks until the session terminates.
//
// Each stream begins with an OpenStream event describing the public
// client. For HTTP tunnels the event also carries any bytes the
// relay already consumed from the public connection while parsing
// the Host header; those bytes are replayed to the local server
// before bidirectional copying begins.
func ServeStreams(session *yamux.Session, localAddr string) error {
	for {
		stream, err := session.AcceptStream()
		if err != nil {
			if err == io.EOF || session.IsClosed() {
				return nil
			}
			return fmt.Errorf("accept stream: %w", err)
		}
		go handleStream(stream, localAddr)
	}
}

func handleStream(stream net.Conn, localAddr string) {
	defer stream.Close()

	open := events.NewOpenStreamEvent()
	if err := open.Read(stream); err != nil {
		log.Printf("read OpenStream: %v", err)
		return
	}

	local, err := net.Dial("tcp4", localAddr)
	if err != nil {
		log.Printf("dial local %s: %v", localAddr, err)
		return
	}
	defer local.Close()

	if len(open.Data.InitialData) > 0 {
		if _, err := local.Write(open.Data.InitialData); err != nil {
			log.Printf("replay initial data: %v", err)
			return
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		events.Bind(stream, local)
	}()
	go func() {
		defer wg.Done()
		events.Bind(local, stream)
	}()
	wg.Wait()

	_ = stream.Close()
	_ = local.Close()
}
