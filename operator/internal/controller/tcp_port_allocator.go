package controller

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	xposv1alpha1 "github.com/ametow/xpos/operator/api/v1alpha1"
)

// TCPPortAllocator hands out cluster-unique public TCP ports from a
// configured contiguous range. Ports must be cluster-unique because
// Gateway API TCPRoute attaches to a Gateway listener at a specific
// port — two TCPRoutes on the same port produce undefined routing.
//
// Allocation is stateless: every call scans the current set of
// Tunnels and picks the lowest unused port in [Min, Max]. This is
// O(N) in the number of tunnels but N is expected to stay small
// (hundreds at most), and the alternative (in-memory bitmap with
// disk recovery) adds complexity we don't need yet. The scan is
// safe under operator restart because the source of truth (the
// Tunnel CRs) is authoritative.
type TCPPortAllocator struct {
	Client client.Client
	Min    int32
	Max    int32
}

// Allocate returns a port that is currently unused by any Tunnel,
// EXCLUDING the Tunnel passed as `self` (whose existing port, if
// any, should be re-used). Returns an error if the range is
// exhausted or misconfigured.
func (a *TCPPortAllocator) Allocate(ctx context.Context, self *xposv1alpha1.Tunnel) (int32, error) {
	if a.Min <= 0 || a.Max < a.Min {
		return 0, fmt.Errorf("invalid TCP port range: [%d, %d]", a.Min, a.Max)
	}

	// Re-use an already-allocated port to keep the value stable
	// across reconciles. The reconciler only calls Allocate when
	// it needs to (re)assign a port, but defensively prefer the
	// existing value if it's still in range.
	if self.Status.TCPPort != nil &&
		*self.Status.TCPPort >= a.Min &&
		*self.Status.TCPPort <= a.Max {
		return *self.Status.TCPPort, nil
	}

	used, err := a.usedPorts(ctx, self)
	if err != nil {
		return 0, err
	}

	for p := a.Min; p <= a.Max; p++ {
		if !used[p] {
			return p, nil
		}
	}
	return 0, fmt.Errorf("TCP port range [%d, %d] exhausted (%d in use)",
		a.Min, a.Max, len(used))
}

// usedPorts returns the set of ports currently claimed by other
// Tunnels (any namespace). `self` is excluded so that re-reconciling
// a tunnel that owns a port doesn't see its own port as "used".
func (a *TCPPortAllocator) usedPorts(ctx context.Context, self *xposv1alpha1.Tunnel) (map[int32]bool, error) {
	var tunnels xposv1alpha1.TunnelList
	if err := a.Client.List(ctx, &tunnels); err != nil {
		return nil, fmt.Errorf("list tunnels: %w", err)
	}
	used := make(map[int32]bool, len(tunnels.Items))
	for i := range tunnels.Items {
		t := &tunnels.Items[i]
		if self != nil && t.UID == self.UID {
			continue
		}
		if t.Status.TCPPort != nil {
			used[*t.Status.TCPPort] = true
		}
	}
	return used, nil
}
