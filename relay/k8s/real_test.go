package k8s

import "testing"

func TestSanitizeForDNS(t *testing.T) {
	cases := map[string]string{
		"Alice":         "alice",
		"alice@bar.com": "alice-bar-com",
		"-bad-":         "bad",
		"UPPER_lower":   "upper-lower",
		"":              "",
	}
	for in, want := range cases {
		if got := sanitizeForDNS(in); got != want {
			t.Errorf("sanitizeForDNS(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAgentName(t *testing.T) {
	got := agentName("Alice", "abcdef1234567890")
	if got != "alice-abcdef12" {
		t.Errorf("agentName = %q", got)
	}
	if got := agentName("", "deadbeef00000000"); got != "anon-deadbeef" {
		t.Errorf("agentName empty identity = %q", got)
	}
}
