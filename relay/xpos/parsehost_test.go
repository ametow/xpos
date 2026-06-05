package xpos

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

// chunkedReader returns its payload in N-byte slices to simulate a
// real network connection where Read can return less than asked for.
// The previous single-Read parser failed against this; the new one
// must succeed.
type chunkedReader struct {
	data   []byte
	off    int
	chunk  int
	closed bool
}

func (c *chunkedReader) Read(p []byte) (int, error) {
	if c.off >= len(c.data) {
		if c.closed {
			return 0, io.EOF
		}
		return 0, io.EOF
	}
	end := c.off + c.chunk
	if end > len(c.data) {
		end = len(c.data)
	}
	n := copy(p, c.data[c.off:end])
	c.off += n
	return n, nil
}

// TestParseHost_HappyPath: a normal GET request with a Host header
// returns the host and a buffer that begins with the request line.
func TestParseHost_HappyPath(t *testing.T) {
	req := "GET /foo HTTP/1.1\r\nHost: alice.example.com\r\nUser-Agent: x\r\n\r\n"
	host, buf, err := parseHost(strings.NewReader(req))
	if err != nil {
		t.Fatalf("parseHost: %v", err)
	}
	if host != "alice.example.com" {
		t.Fatalf("host = %q, want alice.example.com", host)
	}
	if !bytes.HasPrefix(buf, []byte("GET /foo HTTP/1.1\r\n")) {
		t.Fatalf("buf does not start with request line: %q", buf)
	}
}

// TestParseHost_HeaderSplitAcrossReads: the request line and the
// Host header arrive in separate Reads. The old single-Read parser
// returned "no host detected" here; the new one must succeed.
func TestParseHost_HeaderSplitAcrossReads(t *testing.T) {
	req := "GET / HTTP/1.1\r\nHost: split.example.com\r\nAccept: */*\r\n\r\n"
	r := &chunkedReader{data: []byte(req), chunk: 7} // tiny chunks
	host, _, err := parseHost(r)
	if err != nil {
		t.Fatalf("parseHost: %v", err)
	}
	if host != "split.example.com" {
		t.Fatalf("host = %q, want split.example.com", host)
	}
}

// TestParseHost_LargePreamble: a preamble within the cap (8 KiB) is
// fully parsed. This is the regression case for the old 2 KiB
// truncation bug.
func TestParseHost_LargePreamble(t *testing.T) {
	big := strings.Repeat("a", 4096)
	req := "GET / HTTP/1.1\r\nHost: big.example.com\r\nCookie: " + big + "\r\n\r\n"
	host, _, err := parseHost(strings.NewReader(req))
	if err != nil {
		t.Fatalf("parseHost: %v", err)
	}
	if host != "big.example.com" {
		t.Fatalf("host = %q, want big.example.com", host)
	}
}

// TestParseHost_Overflow: a preamble that exceeds the cap and never
// terminates must return a parse error rather than blocking forever
// or truncating silently.
func TestParseHost_Overflow(t *testing.T) {
	// 9 KiB of junk after the request line with no \r\n\r\n.
	req := "GET / HTTP/1.1\r\nHost: late.example.com\r\nX-Filler: " +
		strings.Repeat("z", 9*1024)
	r := &chunkedReader{data: []byte(req), chunk: 1024}
	_, _, err := parseHost(r)
	if err == nil {
		t.Fatalf("expected error for header overflow, got nil")
	}
}

// TestParseHost_EmptyConnection: a peer that connects and closes
// without sending anything must return io.EOF (not a synthetic
// "no host detected") so the gateway can log it accurately.
func TestParseHost_EmptyConnection(t *testing.T) {
	_, _, err := parseHost(strings.NewReader(""))
	if !errors.Is(err, io.EOF) {
		t.Fatalf("err = %v, want io.EOF", err)
	}
}

// TestParseHost_MalformedRequestLine: garbage in must produce a
// parse error and the consumed bytes (so the caller can log them);
// it must NOT panic.
func TestParseHost_MalformedRequestLine(t *testing.T) {
	_, buf, err := parseHost(strings.NewReader("not http\r\n\r\n"))
	if err == nil {
		t.Fatalf("expected error for malformed request")
	}
	if len(buf) == 0 {
		t.Fatalf("buf is empty; caller has no diagnostic bytes")
	}
}

// TestParseHost_HostFromAbsoluteURI: HTTP/1.1 allows the request
// line to carry an absolute URI; net/http.ReadRequest populates
// req.Host from it. The old parser missed this case entirely.
func TestParseHost_HostFromAbsoluteURI(t *testing.T) {
	req := "GET http://abs.example.com/foo HTTP/1.1\r\nHost: abs.example.com\r\n\r\n"
	host, _, err := parseHost(strings.NewReader(req))
	if err != nil {
		t.Fatalf("parseHost: %v", err)
	}
	if host != "abs.example.com" {
		t.Fatalf("host = %q, want abs.example.com", host)
	}
}

// TestParseHost_NoExtraConsumption: the returned buffer length
// matches the bytes actually pulled from the reader. The caller
// forwards `buf` verbatim to the agent; if parseHost consumed more
// from the wire than it returned, the agent would see a truncated
// request.
func TestParseHost_NoExtraConsumption(t *testing.T) {
	req := "GET / HTTP/1.1\r\nHost: exact.example.com\r\n\r\nBODY-BYTES"
	r := &chunkedReader{data: []byte(req), chunk: 16}
	_, buf, err := parseHost(r)
	if err != nil {
		t.Fatalf("parseHost: %v", err)
	}
	// The remaining bytes on the reader plus the returned buf
	// must equal the original payload exactly.
	rest, _ := io.ReadAll(r)
	if got := string(buf) + string(rest); got != req {
		t.Fatalf("round-trip mismatch:\n got: %q\nwant: %q", got, req)
	}
}
