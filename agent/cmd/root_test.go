package cmd

import "testing"

func TestRootCommandStructure(t *testing.T) {
	want := map[string]bool{"auth": false, "http": false, "tcp": false, "version": false}
	for _, command := range rootCmd.Commands() {
		if _, ok := want[command.Name()]; ok {
			want[command.Name()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("root command is missing %q", name)
		}
	}
}

func TestCommandsRequireArguments(t *testing.T) {
	for _, tt := range []struct {
		name string
		args func(cmdArgs []string) error
	}{
		{name: "auth", args: func(args []string) error { return authCommand.Args(authCommand, args) }},
		{name: "http", args: func(args []string) error { return httpCommand.Args(httpCommand, args) }},
		{name: "tcp", args: func(args []string) error { return tcpCommand.Args(tcpCommand, args) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.args(nil); err == nil {
				t.Fatal("Args(nil) error = nil, want missing argument error")
			}
			if err := tt.args([]string{"value"}); err != nil {
				t.Fatalf("Args(one value) error = %v", err)
			}
		})
	}
}
