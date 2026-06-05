package k8s

import (
	"context"
	"errors"
)

// ErrNoTCPPortAllocator is returned by noopClient.WaitTunnelTCPPort
// to signal that no operator-side allocator is available (relay is
// running outside the cluster). Callers should fall back to binding
// an ephemeral port.
var ErrNoTCPPortAllocator = errors.New("no operator-side TCP port allocator (running out-of-cluster)")

// noopClient is returned when the relay is not running in a cluster.
// Every method is a successful no-op so callers don't need nil-checks.
type noopClient struct{}

func (noopClient) Start(ctx context.Context) error { <-ctx.Done(); return nil }
func (noopClient) CreateAgent(context.Context, string, string) (string, error) {
	return "", nil
}
func (noopClient) DeleteAgent(context.Context, string) error                          { return nil }
func (noopClient) CreateTunnel(context.Context, string, string, string, string) error { return nil }
func (noopClient) DeleteTunnel(context.Context, string) error                         { return nil }
func (noopClient) WaitTunnelTCPPort(context.Context, string) (int32, error) {
	return 0, ErrNoTCPPortAllocator
}
func (noopClient) PodName() string   { return "" }
func (noopClient) Namespace() string { return "" }
