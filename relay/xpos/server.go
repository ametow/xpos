package xpos

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ametow/xpos/events"
	"github.com/ametow/xpos/relay/admin"
	"github.com/ametow/xpos/relay/auth"
	"github.com/ametow/xpos/relay/constants"
	"github.com/ametow/xpos/relay/k8s"
	"github.com/ametow/xpos/relay/server"
	"github.com/ametow/xpos/relay/tunnel"
)

type Xpos struct {
	hostname      string
	eventServer   *server.TcpServer
	httpGateway   *server.TcpServer
	httpTunnels   *sync.Map // host -> tunnel.Tunnel
	allTunnels    *sync.Map // tunnel pointer -> tunnel.Tunnel (for shutdown)
	authenticator auth.Authenticator
	logger        *slog.Logger
	admin         *admin.Server
	k8sClient     k8s.Client

	mTunnelsActive *admin.Gauge
	mTunnelsTotal  *admin.Counter
	mAuthFailures  *admin.Counter
	mPublicConns   *admin.Counter
	mFrameErrors   *admin.Counter
}

func New() *Xpos {
	logger := slog.Default()
	adm := admin.New(getenv("XPOS_ADMIN_ADDR", ":9090"), logger)

	kc, err := k8s.NewClient(logger)
	if err != nil {
		logger.Warn("k8s client init failed; running without cluster integration",
			"err", err.Error())
		kc = nil
	}

	return &Xpos{
		hostname:       os.Getenv("XPOS_DOMAIN"),
		eventServer:    server.New(9876, constants.EventServer).WithLogger(logger),
		httpGateway:    server.New(8080, constants.HTTPGateway).WithLogger(logger),
		httpTunnels:    &sync.Map{},
		allTunnels:     &sync.Map{},
		authenticator:  auth.New(),
		logger:         logger,
		admin:          adm,
		k8sClient:      kc,
		mTunnelsActive: adm.Gauge("xpos_tunnels_active"),
		mTunnelsTotal:  adm.Counter("xpos_tunnels_total"),
		mAuthFailures:  adm.Counter("xpos_auth_failures_total"),
		mPublicConns:   adm.Counter("xpos_public_connections_total"),
		mFrameErrors:   adm.Counter("xpos_frame_errors_total"),
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func (x *Xpos) Init() error {
	if x.hostname == "" {
		return errors.New("XPOS_DOMAIN is required")
	}
	if err := x.eventServer.Init(); err != nil {
		return fmt.Errorf("event server init: %w", err)
	}
	if err := x.httpGateway.Init(); err != nil {
		return fmt.Errorf("http gateway init: %w", err)
	}
	return nil
}

// Start runs the relay until ctx is cancelled, then drains. It blocks
// for the lifetime of the server.
func (x *Xpos) Start(ctx context.Context) error {
	var wg sync.WaitGroup
	errCh := make(chan error, 3)

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := x.admin.Start(ctx); err != nil {
			errCh <- fmt.Errorf("admin: %w", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := x.eventServer.Start(ctx, x.handleEventServer); err != nil {
			errCh <- fmt.Errorf("event server: %w", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := x.httpGateway.Start(ctx, x.handleHttpGateway); err != nil {
			errCh <- fmt.Errorf("http gateway: %w", err)
		}
	}()

	// Lease renewal (no-op outside cluster).
	if x.k8sClient != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := x.k8sClient.Start(ctx); err != nil {
				errCh <- fmt.Errorf("k8s lease: %w", err)
			}
		}()
	}

	x.admin.MarkReady()
	x.logger.Info("relay started", "hostname", x.hostname,
		"pod", podOrEmpty(x.k8sClient))

	<-ctx.Done()
	x.admin.MarkNotReady()
	x.logger.Info("shutdown initiated")

	// Close any active tunnels so their goroutines unwind.
	x.allTunnels.Range(func(k, v any) bool {
		v.(tunnel.Tunnel).Close()
		return true
	})

	// Wait for accept loops + handlers to drain (bounded).
	done := make(chan struct{})
	go func() {
		x.eventServer.Wait()
		x.httpGateway.Wait()
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		x.logger.Warn("shutdown drain timeout exceeded")
	}

	close(errCh)
	for err := range errCh {
		if err != nil {
			x.logger.Error("server exited with error", "err", err.Error())
		}
	}
	return nil
}

func (x *Xpos) handleEventServer(ctx context.Context, conn net.Conn) error {
	// Cancel the read loop when the relay is shutting down.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	defer conn.Close()

	req := events.NewTunnelRequestEvent()
	if err := req.Read(conn); err != nil {
		x.mFrameErrors.Inc()
		return err
	}

	if req.Data.Protocol != constants.TCP && req.Data.Protocol != constants.HTTP {
		return events.WriteError(conn, "invalid protocol %s", req.Data.Protocol)
	}

	user, err := x.authenticator.Authenticate(req.Data.AuthToken)
	if err != nil {
		x.mAuthFailures.Inc()
		return events.WriteError(conn, "authentication failed %s", "\n\trequest auth token from https://xpos-it.com/auth\n")
	}

	hostname := fmt.Sprintf("%s.%s", user.Login, x.hostname)
	sessionID := newSessionID()
	logger := x.logger.With(
		"user", user.Login,
		"protocol", req.Data.Protocol,
		"host", hostname,
		"session", sessionID,
	)

	// Register this session with the control plane (no-op out of
	// cluster). We use a detached context for delete so cleanup
	// runs even when ctx has already been cancelled.
	var (
		agentCRName  string
		tunnelCRName string
	)
	if x.k8sClient != nil {
		agentCRName, err = x.k8sClient.CreateAgent(ctx, user.Login, sessionID)
		if err != nil {
			logger.Warn("failed to create Agent CR", "err", err.Error())
		}
		defer func() {
			delCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := x.k8sClient.DeleteAgent(delCtx, agentCRName); err != nil {
				logger.Warn("failed to delete Agent CR", "err", err.Error())
			}
			if err := x.k8sClient.DeleteTunnel(delCtx, tunnelCRName); err != nil {
				logger.Warn("failed to delete Tunnel CR", "err", err.Error())
			}
		}()
	}

	// For TCP tunnels we need the operator to allocate a public
	// port before we can bind, so the order is:
	//   1. create Tunnel CR
	//   2. wait for status.tcpPort
	//   3. Init listener on that port
	// For HTTP tunnels the order is reversed (Init first, then
	// CreateTunnel) because HTTP routing keys on hostname rather
	// than port and doesn't need any operator-side allocation.
	var (
		tn      tunnel.Tunnel
		tcpPort int // 0 = ephemeral fallback (out-of-cluster)
	)
	if x.k8sClient != nil && agentCRName != "" {
		tunnelCRName = fmt.Sprintf("%s-%s", agentCRName, strings.ToLower(req.Data.Protocol))
	}

	switch req.Data.Protocol {
	case constants.HTTP:
		if _, busy := x.httpTunnels.Load(hostname); busy {
			return events.WriteError(conn, "subdomain is busy: %s, try another one", user.Login)
		}
		tn = tunnel.NewHttpTunnel(hostname, conn)
		x.httpTunnels.Store(hostname, tn)
		defer x.httpTunnels.Delete(hostname)
	case constants.TCP:
		if tunnelCRName != "" {
			if err := x.k8sClient.CreateTunnel(ctx, tunnelCRName, hostname, req.Data.Protocol, agentCRName); err != nil {
				logger.Warn("failed to create Tunnel CR", "err", err.Error())
				tunnelCRName = "" // don't try to delete what we didn't create
			} else {
				// Bound at 30s: long enough for a normal
				// operator reconcile, short enough that a
				// stuck handshake can't pin agent resources.
				waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
				port, err := x.k8sClient.WaitTunnelTCPPort(waitCtx, tunnelCRName)
				cancel()
				if err != nil {
					return events.WriteError(conn, "operator did not allocate a TCP port: %s", err.Error())
				}
				tcpPort = int(port)
			}
		}
		tn = tunnel.NewTcpTunnel(conn, hostname, tcpPort)
	default:
		return nil
	}

	if err := tn.Init(ctx); err != nil {
		return events.WriteError(conn, "failed to create tunnel: %s", err.Error())
	}
	x.allTunnels.Store(tn, tn)
	x.mTunnelsActive.Inc()
	x.mTunnelsTotal.Inc()
	defer func() {
		x.allTunnels.Delete(tn)
		x.mTunnelsActive.Dec()
		tn.Close()
	}()

	logger.Info("tunnel created",
		"public", tn.PublicAddr(),
		"private", tn.PrivateAddr(),
	)

	// HTTP: register the Tunnel CR now that Init succeeded. TCP
	// already created it above so the operator could allocate a
	// port.
	if x.k8sClient != nil && agentCRName != "" && req.Data.Protocol == constants.HTTP {
		if err := x.k8sClient.CreateTunnel(ctx, tunnelCRName, hostname, req.Data.Protocol, agentCRName); err != nil {
			logger.Warn("failed to create Tunnel CR", "err", err.Error())
			tunnelCRName = "" // don't try to delete what we didn't create
		}
	}

	tunnelCreatedEvent := events.NewTunnelCreatedEvent()
	tunnelCreatedEvent.Data.Hostname = x.hostname
	tunnelCreatedEvent.Data.PublicListenerPort = tn.PublicAddr()
	tunnelCreatedEvent.Data.PrivateListenerPort = tn.PrivateAddr() // always "" in v2

	// IMPORTANT: write TunnelCreated on the *raw* connection before
	// the tunnel wraps it in a yamux session. After Run() the
	// connection is owned by yamux and direct writes would corrupt
	// frames.
	if err := tunnelCreatedEvent.Write(conn); err != nil {
		return err
	}

	if err := tn.Run(ctx); err != nil {
		return fmt.Errorf("tunnel run: %w", err)
	}

	// Block until the yamux session terminates (agent disconnect
	// or relay shutdown). The context.AfterFunc registered earlier
	// closes the raw conn on ctx cancellation, which propagates
	// into yamux as a session close.
	tn.Wait()
	if ctx.Err() == nil {
		logger.Info("agent disconnected")
	}
	return nil
}

func (x *Xpos) handleHttpGateway(ctx context.Context, con net.Conn) error {
	x.mPublicConns.Inc()
	_ = con.SetReadDeadline(time.Now().Add(3 * time.Second))
	host, buffer, err := parseHost(con)
	if err != nil || host == "" {
		_ = con.Close()
		return err
	}
	_ = con.SetReadDeadline(time.Time{})
	host = strings.ToLower(host)

	tn, ok := x.httpTunnels.Load(host)
	if !ok {
		_ = con.Close()
		return fmt.Errorf("no tunnel for host %q", host)
	}
	httpTunnel, ok := tn.(*tunnel.HttpTunnel)
	if !ok {
		_ = con.Close()
		return errors.New("tunnel is closed")
	}

	httpTunnel.PublicConnHandler(con, buffer)
	return nil
}

// maxHTTPHeaderBytes caps how many bytes parseHost will buffer while
// looking for the end of the HTTP request headers. 8 KiB matches the
// default request-header cap used by net/http.Server and comfortably
// fits the upper end of real-world browser preambles (cookies +
// Sec-CH-UA-* headers tend to be the bloaters).
const maxHTTPHeaderBytes = 8 * 1024

// parseHost reads just enough of a public HTTP connection to identify
// the target Host header, returning that host plus the exact bytes
// consumed so the caller can replay them verbatim to the agent.
//
// The previous implementation issued a single Read of up to 2 KiB
// and scanned the raw bytes for "Host: ". That breaks in two ways:
//   - clients that send the request line in one packet and headers
//     in another (curl --http1.0, some proxies) trip the single-Read
//     assumption and return zero bytes.
//   - large preambles (e.g. browsers with many cookies) overflow the
//     2 KiB buffer and the Host header is silently truncated.
//
// The replacement reads incrementally until the canonical
// end-of-headers sentinel ("\r\n\r\n") appears or maxHTTPHeaderBytes
// is hit, then parses the buffered prefix with net/http. Because the
// parser sees an in-memory copy via bytes.NewReader, no extra bytes
// are consumed from the live connection beyond what we return.
func parseHost(r io.Reader) (string, []byte, error) {
	var buf bytes.Buffer
	tmp := make([]byte, 1024)
	sentinel := []byte("\r\n\r\n")
	for buf.Len() < maxHTTPHeaderBytes {
		n, err := r.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
			if bytes.Contains(buf.Bytes(), sentinel) {
				break
			}
		}
		if err != nil {
			if buf.Len() == 0 {
				return "", nil, err
			}
			// EOF mid-headers: still attempt a parse with
			// whatever we have so callers see a meaningful
			// error rather than a generic io.EOF.
			break
		}
	}

	consumed := buf.Bytes()
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(consumed)))
	if err != nil {
		return "", consumed, fmt.Errorf("parse http request: %w", err)
	}
	if req.Host == "" {
		return "", consumed, fmt.Errorf("no Host header")
	}
	return req.Host, consumed, nil
}

// newSessionID returns a random 16-hex-char session identifier. We
// don't need cryptographic uniqueness, just enough entropy to avoid
// CR-name collisions between concurrent sessions for the same user.
func newSessionID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fall back to a timestamp; uniqueness still holds in
		// practice and we don't want to fail a connect on this.
		return fmt.Sprintf("%016x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// podOrEmpty returns the k8s pod name when running in-cluster, or
// "(none)" otherwise; convenience for log lines.
func podOrEmpty(c k8s.Client) string {
	if c == nil {
		return "(none)"
	}
	if name := c.PodName(); name != "" {
		return name
	}
	return "(none)"
}
