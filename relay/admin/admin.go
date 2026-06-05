// Package admin exposes a small HTTP server with /healthz, /readyz and
// /metrics endpoints intended to back Kubernetes liveness/readiness probes
// and a Prometheus scrape (via the standard text exposition format).
//
// Metrics are kept dependency-free for now: a tiny in-process counter +
// gauge registry that renders the text format. We can swap to
// prometheus/client_golang later without changing call sites.
package admin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type Server struct {
	addr   string
	srv    *http.Server
	ready  atomic.Bool
	reg    *registry
	logger *slog.Logger
}

func New(addr string, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{
		addr:   addr,
		reg:    newRegistry(),
		logger: logger,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/readyz", s.handleReadyz)
	mux.HandleFunc("/metrics", s.handleMetrics)
	s.srv = &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

func (s *Server) MarkReady()    { s.ready.Store(true) }
func (s *Server) MarkNotReady() { s.ready.Store(false) }

func (s *Server) Counter(name string) *Counter { return s.reg.counter(name) }
func (s *Server) Gauge(name string) *Gauge     { return s.reg.gauge(name) }

// Start runs the admin HTTP server until ctx is cancelled, then performs
// a bounded graceful shutdown.
func (s *Server) Start(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("admin server listening", "addr", s.addr)
		if err := s.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.srv.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		return err
	}
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	if !s.ready.Load() {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}

func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	s.reg.write(w)
}

// --- minimal metrics registry ---

type Counter struct{ v atomic.Int64 }

func (c *Counter) Inc()             { c.v.Add(1) }
func (c *Counter) Add(delta int64)  { c.v.Add(delta) }
func (c *Counter) Value() int64     { return c.v.Load() }

type Gauge struct{ v atomic.Int64 }

func (g *Gauge) Inc()           { g.v.Add(1) }
func (g *Gauge) Dec()           { g.v.Add(-1) }
func (g *Gauge) Set(v int64)    { g.v.Store(v) }
func (g *Gauge) Value() int64   { return g.v.Load() }

type registry struct {
	mu       sync.RWMutex
	counters map[string]*Counter
	gauges   map[string]*Gauge
}

func newRegistry() *registry {
	return &registry{
		counters: map[string]*Counter{},
		gauges:   map[string]*Gauge{},
	}
}

func (r *registry) counter(name string) *Counter {
	r.mu.RLock()
	c, ok := r.counters[name]
	r.mu.RUnlock()
	if ok {
		return c
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.counters[name]; ok {
		return c
	}
	c = &Counter{}
	r.counters[name] = c
	return c
}

func (r *registry) gauge(name string) *Gauge {
	r.mu.RLock()
	g, ok := r.gauges[name]
	r.mu.RUnlock()
	if ok {
		return g
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if g, ok := r.gauges[name]; ok {
		return g
	}
	g = &Gauge{}
	r.gauges[name] = g
	return g
}

func (r *registry) write(w http.ResponseWriter) {
	r.mu.RLock()
	cnames := make([]string, 0, len(r.counters))
	for n := range r.counters {
		cnames = append(cnames, n)
	}
	gnames := make([]string, 0, len(r.gauges))
	for n := range r.gauges {
		gnames = append(gnames, n)
	}
	r.mu.RUnlock()
	sort.Strings(cnames)
	sort.Strings(gnames)
	for _, n := range cnames {
		fmt.Fprintf(w, "# TYPE %s counter\n%s %d\n", n, n, r.counters[n].Value())
	}
	for _, n := range gnames {
		fmt.Fprintf(w, "# TYPE %s gauge\n%s %d\n", n, n, r.gauges[n].Value())
	}
}
