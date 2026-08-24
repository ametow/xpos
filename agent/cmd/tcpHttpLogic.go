package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"time"

	"github.com/ametow/xpos/agent/config"
	"github.com/ametow/xpos/agent/handler"
	"github.com/ametow/xpos/events"
)

const (
	initialReconnectDelay = time.Second
	maxReconnectDelay     = 30 * time.Second
)

type permanentError struct{ err error }

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }

type reconnectOptions struct {
	initialDelay time.Duration
	maxDelay     time.Duration
	dial         func(context.Context, string, string) (net.Conn, error)
	wait         func(context.Context, time.Duration) error
	handleConn   func(*events.Event[events.NewConnection], string, string) error
	logf         func(string, ...any)
}

func defaultReconnectOptions() reconnectOptions {
	dialer := &net.Dialer{}
	return reconnectOptions{
		initialDelay: initialReconnectDelay,
		maxDelay:     maxReconnectDelay,
		dial:         dialer.DialContext,
		wait:         waitForRetry,
		handleConn:   handler.HandleConn,
		logf:         log.Printf,
	}
}

func tcpHttpCommand(ctx context.Context, protocol, port string) {
	var conf config.Config
	if err := conf.Load(); err != nil {
		fmt.Println(err)
		return
	}

	if err := reconnect(ctx, conf, protocol, port, defaultReconnectOptions()); err != nil {
		fmt.Println(err)
	}
}

func reconnect(ctx context.Context, conf config.Config, protocol, port string, opts reconnectOptions) error {
	delay := opts.initialDelay
	for {
		connected, err := runTunnelSession(ctx, conf, protocol, port, opts)
		if ctx.Err() != nil {
			return nil
		}
		var permanent permanentError
		if errors.As(err, &permanent) {
			return permanent.err
		}
		if connected {
			delay = opts.initialDelay
		}

		opts.logf("relay connection lost: %v; reconnecting in %s", err, delay)
		if err := opts.wait(ctx, delay); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		delay = nextDelay(delay, opts.maxDelay)
	}
}

func runTunnelSession(ctx context.Context, conf config.Config, protocol, port string, opts reconnectOptions) (bool, error) {
	conn, err := opts.dial(ctx, "tcp4", conf.Remote.Events)
	if err != nil {
		return false, err
	}
	defer conn.Close()
	stopClose := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopClose()

	request := events.NewTunnelRequestEvent()
	request.Data.Protocol = protocol
	request.Data.AuthToken = conf.Local.AuthToken

	if err := request.Write(conn); err != nil {
		return false, err
	}

	tunnelCreated := events.NewTunnelCreatedEvent()
	if err := tunnelCreated.Read(conn); err != nil {
		return false, err
	}
	if tunnelCreated.Data.ErrorMessage != "" {
		return false, permanentError{err: errors.New(tunnelCreated.Data.ErrorMessage)}
	}

	displayProtocol := protocol
	if displayProtocol == "http" {
		displayProtocol = "https"
	}
	localAddr := net.JoinHostPort("127.0.0.1", port)

	fmt.Println("Started listening on public network.")
	fmt.Printf("Protocol: \t %s \n", strings.ToUpper(protocol))
	fmt.Printf("Forwarded: \t %s://%s -> %s \n", displayProtocol, tunnelCreated.Data.PublicListenerPort, localAddr)

	for {
		newConnectionEvent := events.NewConnectionEvent()
		if err := newConnectionEvent.Read(conn); err != nil {
			return true, err
		}

		go func(event *events.Event[events.NewConnection]) {
			if err := opts.handleConn(event, localAddr, tunnelCreated.Data.PrivateListenerPort); err != nil && !errors.Is(err, io.EOF) {
				opts.logf("connection forwarding failed: %v", err)
			}
		}(newConnectionEvent)
	}
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func nextDelay(current, maximum time.Duration) time.Duration {
	if current >= maximum/2 {
		return maximum
	}
	return current * 2
}
