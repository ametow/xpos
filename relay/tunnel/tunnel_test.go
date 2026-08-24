package tunnel

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"github.com/ametow/xpos/events"
)

func TestTcpTunnelPublicAndPrivateHandlers(t *testing.T) {
	agentServer, agentClient := net.Pipe()
	defer agentServer.Close()
	defer agentClient.Close()
	tn := NewTcpTunnel(agentServer, "localhost").(*TcpTunnel)

	publicClient, publicServer := tcpPair(t)
	defer publicClient.Close()
	defer publicServer.Close()

	publicErr := make(chan error, 1)
	go func() { publicErr <- tn.publicConnHandler(publicServer) }()
	event := events.NewConnectionEvent()
	if err := event.Read(agentClient); err != nil {
		t.Fatalf("reading new connection event: %v", err)
	}
	if err := <-publicErr; err != nil {
		t.Fatalf("publicConnHandler() error = %v", err)
	}
	if event.Data.ClientAddr != publicServer.RemoteAddr().String() {
		t.Fatalf("client address = %q, want %q", event.Data.ClientAddr, publicServer.RemoteAddr())
	}

	privateClient, privateServer := net.Pipe()
	defer privateClient.Close()
	defer privateServer.Close()
	privateErr := make(chan error, 1)
	go func() { privateErr <- tn.privConnHandler(privateServer) }()

	addr := publicClient.LocalAddr().(*net.TCPAddr)
	header := make([]byte, 6)
	copy(header, addr.IP.To4())
	binary.LittleEndian.PutUint16(header[4:], uint16(addr.Port))
	if _, err := privateClient.Write(header); err != nil {
		t.Fatal(err)
	}
	assertTunnelData(t, publicClient, privateClient, []byte("public request"))
	assertTunnelData(t, privateClient, publicClient, []byte("private response"))

	_ = publicClient.Close()
	_ = privateClient.Close()
	select {
	case err := <-privateErr:
		if err != nil {
			t.Fatalf("privConnHandler() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("privConnHandler() did not return")
	}
}

func TestPrivateHandlerRejectsUnknownClient(t *testing.T) {
	agentServer, agentClient := net.Pipe()
	defer agentServer.Close()
	defer agentClient.Close()
	tn := NewTcpTunnel(agentServer, "localhost").(*TcpTunnel)
	privateClient, privateServer := net.Pipe()
	defer privateClient.Close()

	errCh := make(chan error, 1)
	go func() { errCh <- tn.privConnHandler(privateServer) }()
	header := []byte{127, 0, 0, 1, 1, 0}
	_, _ = privateClient.Write(header)
	if err := <-errCh; err == nil {
		t.Fatal("privConnHandler() error = nil, want unknown client error")
	}
}

func TestHttpTunnelForwardsInitialBuffer(t *testing.T) {
	agentServer, agentClient := net.Pipe()
	defer agentServer.Close()
	defer agentClient.Close()
	tn := NewHttpTunnel("example.test", agentServer).(*HttpTunnel)
	publicClient, publicServer := tcpPair(t)
	defer publicClient.Close()
	defer publicServer.Close()

	initial := []byte("GET / HTTP/1.1\r\nHost: example.test\r\n\r\n")
	publicDone := make(chan struct{})
	go func() {
		tn.PublicConnHandler(publicServer, initial)
		close(publicDone)
	}()
	event := events.NewConnectionEvent()
	if err := event.Read(agentClient); err != nil {
		t.Fatal(err)
	}
	<-publicDone

	privateClient, privateServer := net.Pipe()
	defer privateClient.Close()
	errCh := make(chan error, 1)
	go func() { errCh <- tn.privConnHandler(privateServer) }()
	addr := publicClient.LocalAddr().(*net.TCPAddr)
	header := make([]byte, 6)
	copy(header, addr.IP.To4())
	binary.LittleEndian.PutUint16(header[4:], uint16(addr.Port))
	_, _ = privateClient.Write(header)

	_ = privateClient.SetReadDeadline(time.Now().Add(2 * time.Second))
	got := make([]byte, len(initial))
	if _, err := io.ReadFull(privateClient, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != string(initial) {
		t.Fatalf("initial buffer = %q, want %q", got, initial)
	}
	_ = publicClient.Close()
	_ = privateClient.Close()
	<-errCh
}

func tcpPair(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	client, err := net.Dial("tcp4", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	server, err := ln.Accept()
	if err != nil {
		client.Close()
		t.Fatal(err)
	}
	return client, server
}

func assertTunnelData(t *testing.T, src, dst net.Conn, payload []byte) {
	t.Helper()
	_ = dst.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := src.Write(payload); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(dst, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("tunnel data = %q, want %q", got, payload)
	}
}
