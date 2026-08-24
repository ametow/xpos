package handler

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"github.com/ametow/xpos/events"
)

func TestHandleConnBridgesLocalAndRemoteConnections(t *testing.T) {
	localListener := listenTCP(t)
	remoteListener := listenTCP(t)
	defer localListener.Close()
	defer remoteListener.Close()

	client := events.NewConnectionEvent()
	client.Data.ClientAddr = "127.0.0.1:43210"
	errCh := make(chan error, 1)
	go func() {
		errCh <- HandleConn(client, localListener.Addr().String(), remoteListener.Addr().String())
	}()

	localPeer := acceptTCP(t, localListener)
	defer localPeer.Close()
	remotePeer := acceptTCP(t, remoteListener)
	defer remotePeer.Close()

	header := make([]byte, 6)
	if _, err := io.ReadFull(remotePeer, header); err != nil {
		t.Fatalf("reading client header: %v", err)
	}
	if got := net.IP(header[:4]).String(); got != "127.0.0.1" {
		t.Fatalf("header IP = %s", got)
	}
	if got := binary.LittleEndian.Uint16(header[4:]); got != 43210 {
		t.Fatalf("header port = %d", got)
	}

	assertForwarded(t, localPeer, remotePeer, []byte("local to remote"))
	assertForwarded(t, remotePeer, localPeer, []byte("remote to local"))

	_ = localPeer.Close()
	_ = remotePeer.Close()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("HandleConn() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("HandleConn() did not return")
	}
}

func TestHandleConnReturnsDialError(t *testing.T) {
	listener := listenTCP(t)
	addr := listener.Addr().String()
	listener.Close()

	client := events.NewConnectionEvent()
	client.Data.ClientAddr = "127.0.0.1:1234"
	if err := HandleConn(client, addr, addr); err == nil {
		t.Fatal("HandleConn() error = nil, want dial error")
	}
}

func listenTCP(t *testing.T) *net.TCPListener {
	t.Helper()
	ln, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

func acceptTCP(t *testing.T, ln *net.TCPListener) net.Conn {
	t.Helper()
	_ = ln.SetDeadline(time.Now().Add(2 * time.Second))
	conn, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func assertForwarded(t *testing.T, src, dst net.Conn, payload []byte) {
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
		t.Fatalf("forwarded data = %q, want %q", got, payload)
	}
}
