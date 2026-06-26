package debugger

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/djherbis/buffer"
	"github.com/djherbis/nio/v3"
)

type Conn interface {
	Request() io.Writer
	Response() io.Writer
	// Close closes the underlying pipes so the parsers receive EOF and
	// flush any final, connection-close-delimited body.
	Close() error
}

type Debugger interface {
	Run(port int) (int, error)
	Connection(id uint64) Conn
}

type conn struct {
	requestReader  io.Reader
	requestWriter  io.WriteCloser
	responseReader io.Reader
	responseWriter io.WriteCloser
}

type debugger struct {
	mu          sync.Mutex
	listeners   map[int64]chan<- interface{}
	connections map[uint64]*conn
}

func New() Debugger {
	d := &debugger{
		listeners:   make(map[int64]chan<- interface{}),
		connections: make(map[uint64]*conn),
	}
	http.HandleFunc("/", contentHandler(html, "text/html"))
	http.HandleFunc("/script.js", contentHandler(js, "text/javascript"))
	http.HandleFunc("/style.css", contentHandler(css, "text/css"))
	http.HandleFunc("/events", d.eventHandler)
	return d
}

func (d *debugger) Run(port int) (int, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return 0, err
	}
	go http.Serve(listener, nil)
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func (c *conn) Request() io.Writer {
	return c.requestWriter
}

func (c *conn) Response() io.Writer {
	return c.responseWriter
}

func (c *conn) Close() error {
	_ = c.requestWriter.Close()
	return c.responseWriter.Close()
}

func (d *debugger) Connection(id uint64) Conn {
	c := &conn{}
	d.mu.Lock()
	d.connections[id] = c
	d.mu.Unlock()
	c.requestReader, c.requestWriter = nio.Pipe(buffer.New(1 << 18))
	c.responseReader, c.responseWriter = nio.Pipe(buffer.New(1 << 18))
	// methods carries each request's method to the response parser so
	// it can frame the matching response body correctly.
	methods := make(chan string, 64)
	go parseRequests(c.requestReader, strconv.FormatUint(id, 10), d.dispatchEvent, methods)
	go parseResponses(c.responseReader, strconv.FormatUint(id, 10), d.dispatchEvent, methods)
	return c
}

func (d *debugger) dispatchEvent(event interface{}) {
	d.mu.Lock()
	listeners := make([]chan<- interface{}, 0, len(d.listeners))
	for _, listener := range d.listeners {
		listeners = append(listeners, listener)
	}
	d.mu.Unlock()
	for _, listener := range listeners {
		go func(l chan<- interface{}) { l <- event }(listener)
	}
}

func (d *debugger) eventHandler(w http.ResponseWriter, r *http.Request) {
	events := make(chan interface{})
	listenerId := time.Now().UnixNano()
	d.mu.Lock()
	d.listeners[listenerId] = events
	d.mu.Unlock()
	defer close(events)
	defer func() {
		d.mu.Lock()
		delete(d.listeners, listenerId)
		d.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(200)

	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-events:
			data, _ := json.Marshal(event)
			content := fmt.Sprintf("data: %s\n\n", string(data))
			w.Write([]byte(content))
			w.(http.Flusher).Flush()
		}
	}
}

//go:embed static/index.html
var html string

//go:embed static/style.css
var css string

//go:embed static/script.js
var js string

func contentHandler(content string, contentType string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.Write([]byte(content))
	}
}
