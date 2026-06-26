package debugger

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

// maxBodySize bounds how many bytes of a request/response body the
// debugger buffers for display. Larger bodies are captured up to this
// limit and the remainder is drained (but not shown).
const maxBodySize = 65536

type request struct {
	Id      string              `json:"id"`
	Method  string              `json:"method"`
	URL     string              `json:"url"`
	Body    string              `json:"body"`
	Headers map[string][]string `json:"headers"`
}

type response struct {
	RequestId string              `json:"request_id"`
	Status    int                 `json:"status"`
	Headers   map[string][]string `json:"headers"`
	// Body is a pointer so a headers-only event (dispatched before the
	// body has been read) omits the field entirely; the frontend then
	// shows the body loader until the follow-up event carries the body.
	Body *string `json:"body,omitempty"`
}

func parseRequests(r io.Reader, conId string, process func(interface{}), methods chan<- string) {
	// Closing lets the response parser fall back to a default method
	// once no more requests will arrive on this connection.
	defer close(methods)
	br := bufio.NewReader(r)
	for i := 0; ; i++ {
		req, err := http.ReadRequest(br)
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
				fmt.Println("[debugger] error parsing http request", err)
			}
			break
		}
		// Hand the method to the response parser so it can frame the
		// matching response body correctly (e.g. HEAD has no body).
		methods <- req.Method
		r := request{
			Id:      conId + "000" + strconv.Itoa(i),
			Method:  req.Method,
			URL:     req.URL.String(),
			Headers: req.Header,
		}

		body, _ := io.ReadAll(io.LimitReader(req.Body, maxBodySize))
		r.Body = string(body)
		// Drain any remainder so the reader is positioned at the next
		// request on a keep-alive connection.
		io.Copy(io.Discard, req.Body)
		process(r)
	}
}

func parseResponses(r io.Reader, conId string, process func(interface{}), methods <-chan string) {
	br := bufio.NewReader(r)
	for i := 0; ; i++ {
		// Recover the originating request's method. http.ReadResponse
		// needs it to know whether a body is present; passing nil makes
		// it mis-frame HEAD responses and desync the whole connection.
		method, ok := <-methods
		if !ok {
			method = http.MethodGet
		}
		resp, err := http.ReadResponse(br, &http.Request{Method: method})
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
				fmt.Println("[debugger] error parsing http response", err)
			}
			return
		}
		id := conId + "000" + strconv.Itoa(i)

		// Emit status + headers immediately so the UI fills them in even
		// before a large or slow (e.g. streaming) body finishes.
		process(response{RequestId: id, Status: resp.StatusCode, Headers: resp.Header})

		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxBodySize))
		// Drain any remainder so the reader is positioned at the next
		// response on a keep-alive connection.
		io.Copy(io.Discard, resp.Body)
		bodyStr := string(body)
		process(response{RequestId: id, Status: resp.StatusCode, Headers: resp.Header, Body: &bodyStr})
	}
}
