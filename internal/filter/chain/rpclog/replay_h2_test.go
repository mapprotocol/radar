package rpclog

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type connKey struct{}

// resetOnSecondRequestH2 serves HTTP/2 over TLS, like cloudflare does for
// cf-rpc, and resets the underlying TCP connection when the second request
// arrives on it. The h2 transport does not retry a bare TCP reset, which is
// why the production resets kept surfacing after the HTTP/1.1 retry fix.
func resetOnSecondRequestH2(t *testing.T) (*httptest.Server, *int32) {
	t.Helper()
	var requests int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = io.Copy(io.Discard, req.Body)
		if atomic.AddInt32(&requests, 1) == 2 {
			raw := req.Context().Value(connKey{}).(*tls.Conn).NetConn().(*net.TCPConn)
			raw.SetLinger(0) // close sends RST
			raw.Close()
			return
		}
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":"0x1"}`)
	}))
	server.EnableHTTP2 = true
	server.Config.ConnContext = func(ctx context.Context, c net.Conn) context.Context {
		return context.WithValue(ctx, connKey{}, c)
	}
	server.StartTLS()
	t.Cleanup(server.Close)
	return server, &requests
}

func postTwiceH2(t *testing.T, client *http.Client, url string) error {
	t.Helper()
	for i := 0; i < 2; i++ {
		req, err := http.NewRequest(http.MethodPost, url,
			strings.NewReader(`{"jsonrpc":"2.0","method":"eth_getLogs","id":1}`))
		if err != nil {
			t.Fatal(err)
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	return nil
}

func h2Base(t *testing.T, server *httptest.Server) *http.Transport {
	t.Helper()
	base := server.Client().Transport.(*http.Transport).Clone()
	t.Cleanup(base.CloseIdleConnections)
	return base
}

func TestPlainH2TransportFailsOnReset(t *testing.T) {
	server, _ := resetOnSecondRequestH2(t)
	client := &http.Client{Transport: h2Base(t, server)}
	err := postTwiceH2(t, client, server.URL)
	if err == nil || !strings.Contains(err.Error(), "connection reset by peer") {
		t.Fatalf("second post error = %v, want connection reset by peer like production", err)
	}
}

func TestTransportRetriesResetOnH2(t *testing.T) {
	server, requests := resetOnSecondRequestH2(t)
	logger := new(recordingLogger)
	client := &http.Client{Transport: NewTransport(h2Base(t, server), logger)}

	if err := postTwiceH2(t, client, server.URL); err != nil {
		t.Fatalf("second post error = %v, want retried after the reset", err)
	}
	if got := atomic.LoadInt32(requests); got != 3 {
		t.Fatalf("server requests = %d, want 3 (ok, reset, retry)", got)
	}

	var completed []logEntry
	resets := 0
	for _, e := range logger.entries {
		switch e.msg {
		case "Chain RPC request completed":
			completed = append(completed, e)
		case "Chain RPC request reset, retrying":
			resets++
		}
	}
	if resets != 1 {
		t.Fatalf("reset notices = %d, want 1", resets)
	}
	if len(completed) != 2 {
		t.Fatalf("completed logs = %d, want 2", len(completed))
	}
	last := completed[1]
	if last.level != "info" || last.fields["status"] != http.StatusOK || last.fields["proto"] != "HTTP/2.0" {
		t.Fatalf("retried request log = %#v, want a successful HTTP/2.0 request", last)
	}
	if got := last.fields["conns"]; got != 2 {
		t.Fatalf("retried request conns = %#v, want 2", got)
	}
}
