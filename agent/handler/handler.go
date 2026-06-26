package handler

import (
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"

	"github.com/hashicorp/yamux"

	"github.com/ametow/xpos/agent/debugger"
	"github.com/ametow/xpos/events"
)

// connCounter assigns a unique, monotonically increasing ID to each
// public connection so the debugger never reuses an ID (the previous
// scheme keyed on the client's ephemeral source port, which collides
// when ports are recycled).
var connCounter uint64

// ServeStreams accepts yamux streams from the relay (each one
// corresponds to a new public connection) and bridges them to the
// local target address. It blocks until the session terminates.
//
// Each stream begins with an OpenStream event describing the public
// client. For HTTP tunnels the event also carries any bytes the
// relay already consumed from the public connection while parsing
// the Host header; those bytes are replayed to the local server
// before bidirectional copying begins.
func ServeStreams(session *yamux.Session, localAddr string, debugg debugger.Debugger) error {
	for {
		stream, err := session.AcceptStream()
		if err != nil {
			if err == io.EOF || session.IsClosed() {
				return nil
			}
			return fmt.Errorf("accept stream: %w", err)
		}
		go handleStream(stream, localAddr, debugg)
	}
}

func handleStream(stream net.Conn, localAddr string, debugg debugger.Debugger) {
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

	debugCon := debugg.Connection(atomic.AddUint64(&connCounter, 1))
	// Closing signals EOF to the debugger parsers so the final
	// connection-close-delimited body gets flushed to the UI.
	defer debugCon.Close()

	if len(open.Data.InitialData) > 0 {
		if _, err := local.Write(open.Data.InitialData); err != nil {
			log.Printf("replay initial data: %v", err)
			return
		}
		// Mirror the prefix the relay already consumed into the
		// debugger so it can parse the full HTTP request.
		_, _ = debugCon.Request().Write(open.Data.InitialData)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		events.Bind(stream, local, debugCon.Request())
	}()
	go func() {
		defer wg.Done()
		events.Bind(local, stream, debugCon.Response())
	}()
	wg.Wait()

	_ = stream.Close()
	_ = local.Close()
}
