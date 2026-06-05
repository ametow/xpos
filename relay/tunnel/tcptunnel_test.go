package tunnel

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/hashicorp/yamux"

	"github.com/ametow/xpos/events"
)

// TestTcpTunnelMultiplex exercises the full v2 multiplex path:
//
//  1. Spin up a "fake agent" backed by a net.Pipe and wrap its side
//     in a yamux client (matching what the real agent does).
//  2. Build a TcpTunnel on the other side of the pipe and run it.
//  3. Dial the tunnel's public listener twice, write distinct
//     payloads, and verify the agent demuxes them onto separate
//     streams with the correct OpenStream metadata.
func TestTcpTunnelMultiplex(t *testing.T) {
	relaySide, agentSide := net.Pipe()
	defer relaySide.Close()
	defer agentSide.Close()

	// Stand up the tunnel on the relay side.
	tn := &TcpTunnel{hostname: "test", AgentConn: relaySide}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := tn.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer tn.Close()

	// Start the agent-side yamux client *concurrently* with Run,
	// because yamux performs a handshake during construction.
	type clientResult struct {
		sess *yamux.Session
		err  error
	}
	clientCh := make(chan clientResult, 1)
	go func() {
		cfg := yamux.DefaultConfig()
		cfg.EnableKeepAlive = false
		cfg.LogOutput = io.Discard
		sess, err := yamux.Client(agentSide, cfg)
		clientCh <- clientResult{sess, err}
	}()

	if err := tn.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	cr := <-clientCh
	if cr.err != nil {
		t.Fatalf("yamux client: %v", cr.err)
	}
	defer cr.sess.Close()

	// On the agent side, accept streams and echo their bodies back
	// after stripping the OpenStream header.
	streamSeen := make(chan string, 2)
	go func() {
		for {
			s, err := cr.sess.AcceptStream()
			if err != nil {
				return
			}
			go func(s net.Conn) {
				defer s.Close()
				open := events.NewOpenStreamEvent()
				if err := open.Read(s); err != nil {
					return
				}
				streamSeen <- open.Data.ClientAddr
				// Echo back whatever payload follows.
				_, _ = io.Copy(s, s)
			}(s)
		}
	}()

	// Dial the public listener twice.
	dial := func() net.Conn {
		c, err := net.Dial("tcp4", tn.pubLn.Addr().String())
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		return c
	}
	c1, c2 := dial(), dial()
	defer c1.Close()
	defer c2.Close()

	if _, err := c1.Write([]byte("hello-1")); err != nil {
		t.Fatalf("c1 write: %v", err)
	}
	if _, err := c2.Write([]byte("hello-22")); err != nil {
		t.Fatalf("c2 write: %v", err)
	}

	echo := func(c net.Conn, want string) {
		_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
		buf := make([]byte, len(want))
		if _, err := io.ReadFull(c, buf); err != nil {
			t.Fatalf("read echo: %v", err)
		}
		if string(buf) != want {
			t.Fatalf("echo = %q, want %q", buf, want)
		}
	}
	echo(c1, "hello-1")
	echo(c2, "hello-22")

	// We should have seen exactly two OpenStream events with the
	// two distinct client addrs.
	got := map[string]bool{}
	for i := 0; i < 2; i++ {
		select {
		case addr := <-streamSeen:
			got[addr] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("missing OpenStream %d", i)
		}
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 distinct OpenStream addrs, got %v", got)
	}
}

// TestTcpTunnelPrivateAddrEmpty enforces the v2 invariant: there is
// no private listener.
func TestTcpTunnelPrivateAddrEmpty(t *testing.T) {
	relaySide, agentSide := net.Pipe()
	defer relaySide.Close()
	defer agentSide.Close()

	tn := &TcpTunnel{hostname: "test", AgentConn: relaySide}
	if err := tn.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer tn.Close()

	if got := tn.PrivateAddr(); got != "" {
		t.Fatalf("PrivateAddr = %q, want empty", got)
	}
	if got := tn.PublicAddr(); got == "" {
		t.Fatalf("PublicAddr is empty")
	}
}

// TestTcpTunnelExplicitPort verifies the v2-with-operator path:
// when the relay receives an allocated port from the operator, the
// public listener binds to exactly that port. This is the contract
// the TCPRoute reconciler relies on — if Init silently fell back to
// a different port, the Gateway listener and the relay would
// disagree and traffic would dead-end.
func TestTcpTunnelExplicitPort(t *testing.T) {
	// Grab a free port via the kernel, close it, then claim it
	// back through the constructor. Racy in theory; in practice
	// fine because no other process on the loopback grabs it
	// between Close and Listen in this short window.
	probe, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("probe listen: %v", err)
	}
	want := probe.Addr().(*net.TCPAddr).Port
	probe.Close()

	relaySide, agentSide := net.Pipe()
	defer relaySide.Close()
	defer agentSide.Close()

	tn := NewTcpTunnel(relaySide, "test", want).(*TcpTunnel)
	if err := tn.Init(context.Background()); err != nil {
		t.Fatalf("Init with port %d: %v", want, err)
	}
	defer tn.Close()

	got := tn.pubLn.Addr().(*net.TCPAddr).Port
	if got != want {
		t.Fatalf("listener bound to port %d, want %d", got, want)
	}
}
