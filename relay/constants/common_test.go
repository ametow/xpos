package constants

import (
	"testing"
	"time"
)

func TestDateTimeCustomLayout(t *testing.T) {
	want := "2026/08/24 12:34:56"
	got := time.Date(2026, 8, 24, 12, 34, 56, 0, time.UTC).Format(DateTimeCustomLayout)
	if got != want {
		t.Fatalf("formatted time = %q, want %q", got, want)
	}
}

func TestProtocolConstants(t *testing.T) {
	if HTTP != "http" || TCP != "tcp" {
		t.Fatalf("protocol constants = %q, %q", HTTP, TCP)
	}
}
