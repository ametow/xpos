package tunnel

import (
	"context"
	"log/slog"
	"net"
	"time"
)

// PendingConnTimeout bounds how long we hold a public-facing accepted
// connection while waiting for the agent to dial back on the private
// listener. Without this, an unresponsive agent leaks file descriptors
// and memory in the tunnel's connection map.
const PendingConnTimeout = 30 * time.Second

// MaxPendingConnections caps the in-flight, not-yet-bridged public
// connections per tunnel. Exceeding it causes new public connections
// to be rejected immediately.
const MaxPendingConnections = 1024

type Tunnel interface {
	PrivateAddr() string
	PublicAddr() string
	// Init reserves any resources whose addresses must be known
	// before the relay writes the TunnelCreated event (e.g. the
	// public TCP listener). It MUST NOT touch the agent control
	// connection — that connection is wrapped in a yamux session
	// by Run, after the relay has written TunnelCreated on it.
	Init(ctx context.Context) error
	// Run wraps the agent control connection in a yamux session
	// and starts any accept loops. It returns immediately; use
	// Wait to block until the session terminates.
	Run(ctx context.Context) error
	// Wait blocks until the underlying agent connection (yamux
	// session) terminates. Used by the server to know when the
	// agent disconnected so it can drop the tunnel from its
	// registry.
	Wait()
	Close()
}

// processListener accepts connections until ctx is cancelled or the
// listener returns an error. When ctx is cancelled the listener is
// closed which makes Accept return promptly.
func processListener(ctx context.Context, ln net.Listener, handler func(net.Conn) error) error {
	stop := context.AfterFunc(ctx, func() { _ = ln.Close() })
	defer stop()
	defer ln.Close()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go func() {
			if err := handler(conn); err != nil {
				slog.Warn("tunnel handler error", "err", err.Error())
				return
			}
		}()
	}
}
