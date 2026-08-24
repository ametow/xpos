package xpos

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseHost(t *testing.T) {
	tests := []struct {
		name    string
		request string
		want    string
	}{
		{name: "standard header", request: "GET / HTTP/1.1\r\nHost: Demo.Example:8080\r\n\r\n", want: "Demo.Example:8080"},
		{name: "lowercase header", request: "GET / HTTP/1.1\r\nhost: demo.example\r\n\r\n", want: "demo.example"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, buffer, err := parseHost(strings.NewReader(tt.request))
			if err != nil {
				t.Fatalf("parseHost() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseHost() host = %q, want %q", got, tt.want)
			}
			if !bytes.Equal(buffer, []byte(tt.request)) {
				t.Fatal("parseHost() did not preserve the request buffer")
			}
		})
	}
}

func TestParseHostErrors(t *testing.T) {
	tests := []string{
		"GET / HTTP/1.1\r\nUser-Agent: test\r\n\r\n",
		"GET / HTTP/1.1\r\nHost: missing-newline",
	}
	for _, request := range tests {
		if _, _, err := parseHost(strings.NewReader(request)); err == nil {
			t.Fatalf("parseHost(%q) error = nil", request)
		}
	}
}
