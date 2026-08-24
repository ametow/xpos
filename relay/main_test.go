package main

import (
	"errors"
	"os"
	"testing"
)

type fakeRelay struct {
	initErr error
	started bool
	closed  bool
}

func (f *fakeRelay) Init() error {
	return f.initErr
}

func (f *fakeRelay) Start() {
	f.started = true
}

func (f *fakeRelay) Close() {
	f.closed = true
}

func TestRunManagesRelayLifecycle(t *testing.T) {
	service := &fakeRelay{}
	interrupt := make(chan os.Signal, 1)
	interrupt <- os.Interrupt

	if err := run(service, interrupt); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !service.started {
		t.Fatal("run() did not start the relay")
	}
	if !service.closed {
		t.Fatal("run() did not close the relay")
	}
}

func TestRunStopsWhenInitFails(t *testing.T) {
	want := errors.New("init failed")
	service := &fakeRelay{initErr: want}

	err := run(service, make(chan os.Signal))
	if !errors.Is(err, want) {
		t.Fatalf("run() error = %v, want %v", err, want)
	}
	if service.started || service.closed {
		t.Fatalf("failed service lifecycle: started=%v closed=%v", service.started, service.closed)
	}
}
