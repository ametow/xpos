package server

import (
	"io"
	"net"
	"testing"
	"time"
)

func TestTcpServerAcceptsConnections(t *testing.T) {
	server := New(0, "test")
	if err := server.Init(); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	handled := make(chan string, 1)
	startErr := make(chan error, 1)
	go func() {
		startErr <- server.Start(func(conn net.Conn) error {
			defer conn.Close()
			data, err := io.ReadAll(conn)
			if err == nil {
				handled <- string(data)
			}
			return err
		})
	}()

	conn, err := net.DialTimeout("tcp4", server.ln.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = conn.Write([]byte("hello"))
	_ = conn.Close()

	select {
	case got := <-handled:
		if got != "hello" {
			t.Fatalf("handler data = %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handler was not called")
	}

	server.Close()
	select {
	case err := <-startErr:
		if err == nil {
			t.Fatal("Start() error = nil after listener close")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start() did not stop after Close()")
	}
}
