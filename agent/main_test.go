package main

import "testing"

func TestMainExecutesCommand(t *testing.T) {
	original := execute
	t.Cleanup(func() { execute = original })

	called := false
	execute = func() { called = true }
	main()

	if !called {
		t.Fatal("main() did not execute the CLI command")
	}
}
