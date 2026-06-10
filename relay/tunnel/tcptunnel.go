package tunnel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"

	"github.com/hashicorp/yamux"

	"github.com/ametow/xpos/events"
)

// TcpTunnel multiplexes public TCP connections over a yamux session
// established on top of the agent control connection. The agent (the
// yamux client) accepts streams; the relay (this side, the yamux
// server) opens a stream for each new public connection and prefixes
// it with an OpenStream event so the agent learns the public client
// address.
//
// In v1 of the protocol, each tunnel needed two ports: a public
// listener and a "private" listener that the agent dialed back into
// for every accepted public connection. v2 collapses that into a
// single yamux session, so PrivateAddr is now an empty string and the
// agent never opens any TCP listener of its own.
type TcpTunnel struct {
	hostname    string
	AgentConn   net.Conn
	publicAddr  string
	privateAddr string
	desiredPort int

	session *yamux.Session
	pubLn   net.Listener
	pending atomic.Int64
	cancel  context.CancelFunc
}

// NewTcpTunnel constructs a TcpTunnel that, on Init, will bind the
// public listener to `port`. Pass 0 to request an ephemeral port from
// the kernel (used when running outside a cluster, where the operator
// is not allocating ports). In-cluster the relay obtains a stable port
// from the Tunnel CR's status.tcpPort before calling NewTcpTunnel, so
// the bind matches the TCPRoute the operator has reconciled.
func NewTcpTunnel(conn net.Conn, hostname string, port int) Tunnel {
	return &TcpTunnel{
		hostname:    hostname,
		AgentConn:   conn,
		desiredPort: port,
	}
}

func (tn *TcpTunnel) PrivateAddr() string { return tn.privateAddr }
func (tn *TcpTunnel) PublicAddr() string  { return tn.publicAddr }

// Init binds the public listener so PublicAddr is available, but
// does NOT yet wrap the agent connection in yamux. That wrap must
// happen after the relay has finished writing the TunnelCreated event
// on the raw connection. Run() does the wrap and starts the accept
// loop.
func (tn *TcpTunnel) Init(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", tn.desiredPort)
	pubLn, err := net.Listen("tcp4", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	tn.pubLn = pubLn
	tn.publicAddr = fmt.Sprintf("%s:%d", tn.hostname, pubLn.Addr().(*net.TCPAddr).Port)
	return nil
}

// Run wraps the agent connection in a yamux session and starts the
// public accept loop. It returns immediately after wrap; use Wait to
// block until the session terminates.
func (tn *TcpTunnel) Run(ctx context.Context) error {
	if err := tn.startSession(); err != nil {
		return err
	}
	childCtx, cancel := context.WithCancel(ctx)
	tn.cancel = cancel
	go processListener(childCtx, tn.pubLn, tn.handlePublicConn)
	return nil
}

// startSession wraps the agent control connection in a yamux server.
// Subsequent calls on AgentConn must go through tn.session.
func (tn *TcpTunnel) startSession() error {
	cfg := yamux.DefaultConfig()
	// Disable yamux's keepalives — the underlying TCP stack +
	// kube-proxy already handle dead-peer detection, and keepalives
	// can interfere with our own framing.
	cfg.EnableKeepAlive = false
	// Yamux logs to stderr by default; reroute to slog.
	cfg.LogOutput = io.Discard
	sess, err := yamux.Server(tn.AgentConn, cfg)
	if err != nil {
		return fmt.Errorf("yamux server: %w", err)
	}
	tn.session = sess
	return nil
}

// handlePublicConn opens a new yamux stream, writes an OpenStream
// event with the public client's address, then bridges the public
// connection and the stream bidirectionally.
func (tn *TcpTunnel) handlePublicConn(pub net.Conn) error {
	if tn.pending.Load() >= MaxPendingConnections {
		_ = pub.Close()
		return errors.New("tunnel pending-conn limit reached")
	}
	tn.pending.Add(1)
	defer tn.pending.Add(-1)

	stream, err := tn.session.OpenStream()
	if err != nil {
		_ = pub.Close()
		return fmt.Errorf("open stream: %w", err)
	}

	open := events.NewOpenStreamEvent()
	open.Data.ClientAddr = pub.RemoteAddr().String()
	if err := open.Write(stream); err != nil {
		_ = stream.Close()
		_ = pub.Close()
		return fmt.Errorf("write OpenStream: %w", err)
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
	return nil
}

// Wait blocks until the yamux session is closed (either side hung up
// or the relay called Close). The relay's per-tunnel goroutine in
// xpos/server.go uses this to know when the agent disconnected.
func (tn *TcpTunnel) Wait() {
	if tn.session == nil {
		return
	}
	<-tn.session.CloseChan()
}

func (tn *TcpTunnel) Close() {
	if tn.cancel != nil {
		tn.cancel()
	}
	if tn.session != nil {
		_ = tn.session.Close()
	}
	if tn.pubLn != nil {
		_ = tn.pubLn.Close()
	}
}
