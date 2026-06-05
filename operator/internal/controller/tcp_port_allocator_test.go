package controller

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	xposv1alpha1 "github.com/ametow/xpos/operator/api/v1alpha1"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := xposv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("AddToScheme: %v", err)
	}
	return s
}

func tunnel(name string, port *int32) *xposv1alpha1.Tunnel {
	return &xposv1alpha1.Tunnel{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			UID:       types.UID(name),
		},
		Spec: xposv1alpha1.TunnelSpec{
			Protocol: xposv1alpha1.TunnelProtocolTCP,
			Hostname: name + ".example",
			AgentRef: xposv1alpha1.AgentReference{Name: "a"},
		},
		Status: xposv1alpha1.TunnelStatus{TCPPort: port},
	}
}

func ptr(v int32) *int32 { return &v }

// TestAllocator_FirstFreeFromBottom verifies the allocator returns
// the lowest unused port in the configured range, scanning across
// existing Tunnel status.
func TestAllocator_FirstFreeFromBottom(t *testing.T) {
	scheme := newScheme(t)
	cli := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(tunnel("t1", ptr(30000)), tunnel("t2", ptr(30002))).
		Build()

	a := &TCPPortAllocator{Client: cli, Min: 30000, Max: 30005}
	got, err := a.Allocate(context.Background(), tunnel("t3", nil))
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if got != 30001 {
		t.Fatalf("expected 30001, got %d", got)
	}
}

// TestAllocator_ReusesExistingPort: when the reconciled tunnel
// already has a port, the allocator must return it unchanged so
// status doesn't flap and downstream TCPRoute updates are idempotent.
func TestAllocator_ReusesExistingPort(t *testing.T) {
	scheme := newScheme(t)
	self := tunnel("self", ptr(30050))
	cli := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(self, tunnel("other", ptr(30000))).
		Build()

	a := &TCPPortAllocator{Client: cli, Min: 30000, Max: 30099}
	got, err := a.Allocate(context.Background(), self)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if got != 30050 {
		t.Fatalf("expected 30050, got %d", got)
	}
}

// TestAllocator_RangeExhausted returns a clear error rather than a
// nonsensical port when no slot is free.
func TestAllocator_RangeExhausted(t *testing.T) {
	scheme := newScheme(t)
	cli := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			tunnel("t1", ptr(30000)),
			tunnel("t2", ptr(30001)),
		).
		Build()

	a := &TCPPortAllocator{Client: cli, Min: 30000, Max: 30001}
	_, err := a.Allocate(context.Background(), tunnel("t3", nil))
	if err == nil {
		t.Fatalf("expected exhaustion error, got nil")
	}
}

// TestAllocator_InvalidRange catches misconfiguration early.
func TestAllocator_InvalidRange(t *testing.T) {
	scheme := newScheme(t)
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()
	for _, tc := range []struct{ min, max int32 }{
		{0, 100},
		{30100, 30000},
		{-1, 10},
	} {
		a := &TCPPortAllocator{Client: cli, Min: tc.min, Max: tc.max}
		if _, err := a.Allocate(context.Background(), tunnel("t", nil)); err == nil {
			t.Fatalf("expected error for range [%d, %d]", tc.min, tc.max)
		}
	}
}

// TestAllocator_OutOfRangeExistingPort: if a tunnel's status carries
// a port outside the current allowed range (e.g. operator reconfigured),
// allocate a fresh one rather than reusing the stale value.
func TestAllocator_OutOfRangeExistingPort(t *testing.T) {
	scheme := newScheme(t)
	self := tunnel("self", ptr(40000)) // outside the new range
	cli := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(self).
		Build()

	a := &TCPPortAllocator{Client: cli, Min: 30000, Max: 30099}
	got, err := a.Allocate(context.Background(), self)
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if got != 30000 {
		t.Fatalf("expected 30000, got %d", got)
	}
}
