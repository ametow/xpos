package tunnel

import (
	"context"
	"net"
	"sync"

	"github.com/ametow/xpos/events"
)

// HttpTunnel reuses TcpTunnel's yamux session machinery but exposes
// no public listener of its own — public HTTP traffic is dispatched
// to it from the relay's shared HTTP gateway via PublicConnHandler.
//
// The relay reads the leading bytes of each public HTTP request to
// extract the Host header before identifying the right tunnel; those
// already-consumed bytes are forwarded to the agent inside the
// OpenStream event's InitialData field so the agent can replay them
// to the local HTTP server.
type HttpTunnel struct {
	TcpTunnel
}

func NewHttpTunnel(hostname string, conn net.Conn) Tunnel {
	tn := &HttpTunnel{}
	tn.TcpTunnel.hostname = hostname
	tn.TcpTunnel.AgentConn = conn
	tn.TcpTunnel.publicAddr = hostname
	return tn
}

// Init is a no-op: HTTP tunnels have no per-tunnel public listener
// (the relay's HTTP gateway dispatches by hostname), and yamux
// session setup is deferred to Run so the relay can write
// TunnelCreated on the raw connection first.
func (tn *HttpTunnel) Init(ctx context.Context) error { return nil }

// Run wraps the agent connection in yamux. Returns immediately; use
// Wait to block on the session.
func (tn *HttpTunnel) Run(ctx context.Context) error {
	if err := tn.startSession(); err != nil {
		return err
	}
	_, cancel := context.WithCancel(ctx)
	tn.cancel = cancel
	return nil
}

// PublicConnHandler is invoked by the relay's HTTP gateway once it
// has identified that a public connection belongs to this tunnel.
// `prefix` is the bytes already read from the connection during
// host-header parsing; they are sent to the agent verbatim via the
// OpenStream event so the agent can replay them to the local server.
func (tn *HttpTunnel) PublicConnHandler(pub net.Conn, prefix []byte) {
	if tn.session == nil {
		_ = pub.Close()
		return
	}
	if tn.pending.Load() >= MaxPendingConnections {
		_ = pub.Close()
		return
	}
	tn.pending.Add(1)
	defer tn.pending.Add(-1)

	stream, err := tn.session.OpenStream()
	if err != nil {
		_ = pub.Close()
		return
	}

	open := events.NewOpenStreamEvent()
	open.Data.ClientAddr = pub.RemoteAddr().String()
	open.Data.InitialData = prefix
	if err := open.Write(stream); err != nil {
		_ = stream.Close()
		_ = pub.Close()
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		events.Bind(stream, pub)
	}()
	go func() {
		defer wg.Done()
		events.Bind(pub, stream)
	}()
	wg.Wait()

	_ = stream.Close()
	_ = pub.Close()
}

// Compile-time guard: HttpTunnel must satisfy Tunnel.
var _ Tunnel = (*HttpTunnel)(nil)
