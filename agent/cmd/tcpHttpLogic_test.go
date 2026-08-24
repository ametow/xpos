package cmd

import (
	"context"
	"errors"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ametow/xpos/agent/config"
	"github.com/ametow/xpos/events"
)

func TestReconnectUsesExponentialBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var delays []time.Duration
	opts := testReconnectOptions()
	opts.dial = func(context.Context, string, string) (net.Conn, error) {
		return nil, errors.New("relay unavailable")
	}
	opts.wait = func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		if len(delays) == 6 {
			cancel()
			return context.Canceled
		}
		return nil
	}

	if err := reconnect(ctx, config.Config{}, "tcp", "8080", opts); err != nil {
		t.Fatalf("reconnect() error = %v", err)
	}
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 10 * time.Second, 10 * time.Second}
	if !reflect.DeepEqual(delays, want) {
		t.Fatalf("retry delays = %v, want %v", delays, want)
	}
}

func TestReconnectResetsBackoffAfterSuccessfulConnection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	attempt := 0
	opts := testReconnectOptions()
	opts.dial = func(context.Context, string, string) (net.Conn, error) {
		attempt++
		if attempt <= 2 {
			return nil, errors.New("relay unavailable")
		}
		client, relay := net.Pipe()
		go serveTunnelHandshake(relay, "")
		return client, nil
	}
	var delays []time.Duration
	opts.wait = func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		if len(delays) == 3 {
			cancel()
			return context.Canceled
		}
		return nil
	}

	if err := reconnect(ctx, config.Config{}, "tcp", "8080", opts); err != nil {
		t.Fatalf("reconnect() error = %v", err)
	}
	want := []time.Duration{time.Second, 2 * time.Second, time.Second}
	if !reflect.DeepEqual(delays, want) {
		t.Fatalf("retry delays = %v, want %v", delays, want)
	}
}

func TestReconnectStopsOnPermanentRelayError(t *testing.T) {
	opts := testReconnectOptions()
	opts.dial = func(context.Context, string, string) (net.Conn, error) {
		client, relay := net.Pipe()
		go serveTunnelHandshake(relay, "authentication failed")
		return client, nil
	}
	opts.wait = func(context.Context, time.Duration) error {
		t.Fatal("permanent errors must not be retried")
		return nil
	}

	err := reconnect(context.Background(), config.Config{}, "tcp", "8080", opts)
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("reconnect() error = %v, want authentication failure", err)
	}
}

func TestReconnectStopsActiveSessionOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	opts := testReconnectOptions()
	ready := make(chan struct{})
	opts.dial = func(context.Context, string, string) (net.Conn, error) {
		client, relay := net.Pipe()
		go func() {
			defer relay.Close()
			request := events.NewTunnelRequestEvent()
			_ = request.Read(relay)
			created := events.NewTunnelCreatedEvent()
			created.Data.PublicListenerPort = "example.test:1234"
			created.Data.PrivateListenerPort = "127.0.0.1:5678"
			_ = created.Write(relay)
			close(ready)
			<-ctx.Done()
		}()
		return client, nil
	}

	done := make(chan error, 1)
	go func() { done <- reconnect(ctx, config.Config{}, "tcp", "8080", opts) }()
	<-ready
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("reconnect() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("reconnect did not stop after cancellation")
	}
}

func TestNextDelay(t *testing.T) {
	for _, tt := range []struct {
		current time.Duration
		want    time.Duration
	}{
		{time.Second, 2 * time.Second},
		{8 * time.Second, 10 * time.Second},
		{10 * time.Second, 10 * time.Second},
	} {
		if got := nextDelay(tt.current, 10*time.Second); got != tt.want {
			t.Errorf("nextDelay(%s) = %s, want %s", tt.current, got, tt.want)
		}
	}
}

func testReconnectOptions() reconnectOptions {
	return reconnectOptions{
		initialDelay: time.Second,
		maxDelay:     10 * time.Second,
		handleConn: func(*events.Event[events.NewConnection], string, string) error {
			return nil
		},
		logf: func(string, ...any) {},
	}
}

func serveTunnelHandshake(conn net.Conn, errorMessage string) {
	defer conn.Close()
	request := events.NewTunnelRequestEvent()
	if request.Read(conn) != nil {
		return
	}
	created := events.NewTunnelCreatedEvent()
	created.Data.PublicListenerPort = "example.test:1234"
	created.Data.PrivateListenerPort = "127.0.0.1:5678"
	created.Data.ErrorMessage = errorMessage
	_ = created.Write(conn)
}
