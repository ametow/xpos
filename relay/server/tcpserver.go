package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
)

type TcpServer struct {
	name   string
	ln     net.Listener
	port   uint16
	logger *slog.Logger
	wg     sync.WaitGroup
}

func New(port uint16, name string) *TcpServer {
	return &TcpServer{port: port, name: name, logger: slog.Default()}
}

func (s *TcpServer) WithLogger(l *slog.Logger) *TcpServer {
	if l != nil {
		s.logger = l
	}
	return s
}

// Close shuts the underlying listener. Callers should also wait via Wait()
// for in-flight handler goroutines to drain.
func (s *TcpServer) Close() {
	if s.ln != nil {
		_ = s.ln.Close()
	}
}

// Wait blocks until all handler goroutines launched by Start have returned.
func (s *TcpServer) Wait() { s.wg.Wait() }

func (s *TcpServer) Init() error {
	ln, err := net.Listen("tcp4", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return err
	}
	s.ln = ln
	s.logger.Info("listener up", "server", s.name, "addr", ln.Addr().String())
	return nil
}

// Start runs the accept loop until ctx is cancelled or the listener errors.
// When ctx is cancelled the listener is closed and Accept returns an error
// which is treated as a clean shutdown signal.
func (s *TcpServer) Start(ctx context.Context, handler func(context.Context, net.Conn) error) error {
	stop := context.AfterFunc(ctx, func() { _ = s.ln.Close() })
	defer stop()

	for {
		conn, err := s.ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return err
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			if err := handler(ctx, conn); err != nil {
				s.logger.Warn("handler error", "server", s.name, "err", err.Error())
			}
		}()
	}
}
