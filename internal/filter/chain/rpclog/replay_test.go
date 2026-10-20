package rpclog

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

// resetOnReuseServer answers the first request on its first connection, then
// resets that connection when the client reuses it, like a cloudflare pop that
// does not know a flow anycast moved to it. Later connections are served normally.
func resetOnReuseServer(t *testing.T) (url string, conns *int32) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	conns = new(int32)

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			first := atomic.AddInt32(conns, 1) == 1
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				for served := 0; ; served++ {
					req, err := http.ReadRequest(r)
					if err != nil {
						return
					}
					_, _ = io.Copy(io.Discard, req.Body)
					if first && served == 1 {
						c.(*net.TCPConn).SetLinger(0) // close sends RST
						return
					}
					_, _ = c.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok"))
				}
			}(c)
		}
	}()
	return "http://" + ln.Addr().String(), conns
}

func postTwice(t *testing.T, rt http.RoundTripper, url string) error {
	t.Helper()
	client := &http.Client{Transport: rt}
	for i := 0; i < 2; i++ {
		// a body without GetBody, like the xrp client sends
		body := io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","method":"eth_blockNumber","id":1}`))
		resp, err := client.Post(url, "application/json", body)
		if err != nil {
			return err
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	return nil
}

func TestTransportRetriesResetOnReusedConn(t *testing.T) {
	url, conns := resetOnReuseServer(t)
	logger := new(recordingLogger)
	base := &http.Transport{}
	defer base.CloseIdleConnections()

	if err := postTwice(t, NewTransport(base, logger), url); err != nil {
		t.Fatalf("second post error = %v, want retried on a new connection", err)
	}
	if got := atomic.LoadInt32(conns); got != 2 {
		t.Fatalf("server connections = %d, want 2", got)
	}
	if len(logger.entries) != 2 {
		t.Fatalf("log count = %d, want 2", len(logger.entries))
	}
	// the retried request logs both connections it used
	if got := logger.entries[1].fields["conns"]; got != 2 {
		t.Fatalf("second request conns = %#v, want 2", got)
	}
	if got := logger.entries[0].fields["conns"]; got != 1 {
		t.Fatalf("first request conns = %#v, want 1", got)
	}
}

func TestPlainTransportFailsResetOnReusedConn(t *testing.T) {
	url, _ := resetOnReuseServer(t)
	base := &http.Transport{}
	defer base.CloseIdleConnections()
	if err := postTwice(t, base, url); err == nil {
		t.Fatal("second post error = nil, want connection reset without the logging transport")
	}
}

func TestTransportDoesNotSendIdempotencyHeader(t *testing.T) {
	var got http.Header
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
	})}
	go srv.Serve(ln)
	defer srv.Close()

	client := &http.Client{Transport: NewTransport(&http.Transport{}, new(recordingLogger))}
	resp, err := client.Post("http://"+ln.Addr().String(), "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if _, ok := got["X-Idempotency-Key"]; ok {
		t.Fatalf("server saw X-Idempotency-Key header: %v", got)
	}
}
