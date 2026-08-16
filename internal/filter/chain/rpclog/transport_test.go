package rpclog

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestRPCMethods(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "single method",
			body: `{"jsonrpc":"2.0","method":"eth_blockNumber","id":1}`,
			want: "eth_blockNumber",
		},
		{
			name: "batch methods retain order",
			body: `[{"jsonrpc":"2.0","method":"eth_getLogs","id":1},{"jsonrpc":"2.0","method":"eth_blockNumber","id":2}]`,
			want: "eth_getLogs,eth_blockNumber",
		},
		{
			name: "missing method",
			body: `{"jsonrpc":"2.0","id":1}`,
			want: unknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newReplayableRequest(t, tt.body)
			if got := rpcMethods(req); got != tt.want {
				t.Fatalf("rpcMethods() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRPCMethodsNonReplayableBody(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://rpc.example.com", io.NopCloser(strings.NewReader(`{"method":"eth_blockNumber"}`)))
	if err != nil {
		t.Fatal(err)
	}

	if got := rpcMethods(req); got != unknown {
		t.Fatalf("rpcMethods() = %q, want %q", got, unknown)
	}
}

func TestRPCMethodsDoesNotConsumeRequestBody(t *testing.T) {
	body := `{"jsonrpc":"2.0","method":"eth_blockNumber","id":1}`
	req := newReplayableRequest(t, body)

	if got := rpcMethods(req); got != "eth_blockNumber" {
		t.Fatalf("rpcMethods() = %q, want %q", got, "eth_blockNumber")
	}

	gotBody, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(gotBody); got != body {
		t.Fatalf("request body = %q, want %q", got, body)
	}
}

func TestSanitizeEndpoint(t *testing.T) {
	endpoint, err := url.Parse("https://user:secret@rpc.example.com:8545/v1/key?token=secret#fragment")
	if err != nil {
		t.Fatal(err)
	}

	if got := sanitizeEndpoint(endpoint); got != "https://rpc.example.com:8545/v1/key" {
		t.Fatalf("sanitizeEndpoint() = %q", got)
	}
}

func TestSanitizeEndpointInvalidURLs(t *testing.T) {
	tests := []struct {
		name     string
		endpoint *url.URL
	}{
		{name: "nil URL"},
		{name: "websocket URL", endpoint: mustParseURL(t, "ws://rpc.example.com")},
		{name: "missing host", endpoint: mustParseURL(t, "https:/v1/key")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeEndpoint(tt.endpoint); got != unknown {
				t.Fatalf("sanitizeEndpoint() = %q, want %q", got, unknown)
			}
		})
	}
}

func TestTransportLogsCompletedRequest(t *testing.T) {
	const requestBody = `{"jsonrpc":"2.0","method":"eth_blockNumber","id":1}`
	req, err := http.NewRequest(
		http.MethodPost,
		"https://user:secret@rpc.example.com/v1/key?token=secret#fragment",
		strings.NewReader(requestBody),
	)
	if err != nil {
		t.Fatal(err)
	}

	var bodySeen string
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		bodySeen = string(body)
		return &http.Response{
			StatusCode: http.StatusOK,
			Proto:      "HTTP/1.1",
			Body:       io.NopCloser(strings.NewReader(`{}`)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})
	logger := new(recordingLogger)
	transport := NewTransport(base, logger)
	times := []time.Time{time.Unix(0, 0), time.Unix(0, int64(128*time.Millisecond))}
	transport.now = func() time.Time {
		now := times[0]
		times = times[1:]
		return now
	}

	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if bodySeen != requestBody {
		t.Fatalf("wrapped transport body = %q, want %q", bodySeen, requestBody)
	}
	if len(logger.entries) != 1 {
		t.Fatalf("log count = %d, want 1", len(logger.entries))
	}

	entry := logger.entries[0]
	if entry.level != "info" || entry.msg != "Chain RPC request completed" {
		t.Fatalf("completion log = %#v", entry)
	}
	wantFields := map[string]interface{}{
		"http_method":   http.MethodPost,
		"rpc_method":    "eth_blockNumber",
		"endpoint":      "https://rpc.example.com/v1/key",
		"status":        http.StatusOK,
		"duration":      128 * time.Millisecond,
		"conn_wait":     time.Duration(0),
		"dns":           time.Duration(0),
		"tcp_connect":   time.Duration(0),
		"tls_handshake": time.Duration(0),
		"server_wait":   time.Duration(0),
		"reused":        false,
		"was_idle":      false,
		"idle_time":     time.Duration(0),
		"proto":         "HTTP/1.1",
	}
	for key, want := range wantFields {
		if got := entry.fields[key]; got != want {
			t.Errorf("field %s = %#v, want %#v", key, got, want)
		}
	}
}

func TestTransportLogsRequestError(t *testing.T) {
	wantErr := errors.New("dial failed")
	req, err := http.NewRequest(http.MethodPost, "https://rpc.example.com", strings.NewReader(`{"method":"ledger"}`))
	if err != nil {
		t.Fatal(err)
	}
	logger := new(recordingLogger)
	transport := NewTransport(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, wantErr
	}), logger)

	resp, gotErr := transport.RoundTrip(req)
	if resp != nil || !errors.Is(gotErr, wantErr) {
		t.Fatalf("RoundTrip() = (%v, %v), want (nil, %v)", resp, gotErr, wantErr)
	}
	if len(logger.entries) != 1 {
		t.Fatalf("log count = %d, want 1", len(logger.entries))
	}
	entry := logger.entries[0]
	loggedErr, ok := entry.fields["err"].(error)
	if entry.level != "error" || entry.fields["status"] != 0 || !ok || !errors.Is(loggedErr, wantErr) {
		t.Fatalf("error log = %#v", entry)
	}
	wantTraceFields := map[string]interface{}{
		"conn_wait":     time.Duration(0),
		"dns":           time.Duration(0),
		"tcp_connect":   time.Duration(0),
		"tls_handshake": time.Duration(0),
		"server_wait":   time.Duration(0),
		"reused":        false,
		"was_idle":      false,
		"idle_time":     time.Duration(0),
		"proto":         unknown,
	}
	for key, want := range wantTraceFields {
		if got := entry.fields[key]; got != want {
			t.Errorf("field %s = %#v, want %#v", key, got, want)
		}
	}
}

func TestNewHTTPClientWrapsSuppliedTransport(t *testing.T) {
	base := new(noContentRoundTripper)

	client := NewHTTPClient(7*time.Second, base)
	if client.Timeout != 7*time.Second {
		t.Fatalf("timeout = %v, want %v", client.Timeout, 7*time.Second)
	}
	transport, ok := client.Transport.(*Transport)
	if !ok || transport.base != base {
		t.Fatalf("transport = %T, want logging wrapper around supplied base", client.Transport)
	}
}

func TestTransportLogsRealHTTPTrace(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		defer req.Body.Close()
		_, _ = io.Copy(io.Discard, req.Body)
		time.Sleep(25 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":"0x1"}`)
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()

	base, ok := server.Client().Transport.(*http.Transport)
	if !ok {
		t.Fatalf("server transport = %T, want *http.Transport", server.Client().Transport)
	}
	base = base.Clone()
	defer base.CloseIdleConnections()

	logger := new(recordingLogger)
	client := &http.Client{Transport: NewTransport(base, logger)}
	for range 2 {
		req, err := http.NewRequest(
			http.MethodPost,
			server.URL+"/filter/test",
			strings.NewReader(`{"jsonrpc":"2.0","method":"eth_blockNumber","id":1}`),
		)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, readErr := io.Copy(io.Discard, resp.Body)
		closeErr := resp.Body.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("response body errors = (%v, %v)", readErr, closeErr)
		}
	}

	if len(logger.entries) != 2 {
		t.Fatalf("log count = %d, want 2", len(logger.entries))
	}
	fresh, reused := logger.entries[0], logger.entries[1]
	if fresh.fields["proto"] != "HTTP/2.0" || fresh.fields["reused"] != false {
		t.Fatalf("fresh connection metadata = %#v", fresh.fields)
	}
	if traceDuration(t, fresh, "tls_handshake") <= 0 || traceDuration(t, fresh, "server_wait") < 20*time.Millisecond {
		t.Fatalf("fresh connection timings = %#v", fresh.fields)
	}
	if reused.fields["proto"] != "HTTP/2.0" || reused.fields["reused"] != true || reused.fields["was_idle"] != true {
		t.Fatalf("reused connection metadata = %#v", reused.fields)
	}
	if traceDuration(t, reused, "tls_handshake") != 0 || traceDuration(t, reused, "server_wait") < 20*time.Millisecond {
		t.Fatalf("reused connection timings = %#v", reused.fields)
	}

	logTraceEntry(t, "fresh", fresh)
	logTraceEntry(t, "reused", reused)
}

func traceDuration(t *testing.T, entry logEntry, field string) time.Duration {
	t.Helper()
	duration, ok := entry.fields[field].(time.Duration)
	if !ok {
		t.Fatalf("field %s = %#v, want time.Duration", field, entry.fields[field])
	}
	return duration
}

func logTraceEntry(t *testing.T, label string, entry logEntry) {
	t.Helper()
	t.Logf("%s msg=%q http_method=%v rpc_method=%v endpoint=%v status=%v duration=%v conn_wait=%v dns=%v tcp_connect=%v tls_handshake=%v server_wait=%v reused=%v was_idle=%v idle_time=%v proto=%v",
		label, entry.msg, entry.fields["http_method"], entry.fields["rpc_method"],
		entry.fields["endpoint"], entry.fields["status"], entry.fields["duration"],
		entry.fields["conn_wait"], entry.fields["dns"], entry.fields["tcp_connect"],
		entry.fields["tls_handshake"], entry.fields["server_wait"], entry.fields["reused"],
		entry.fields["was_idle"], entry.fields["idle_time"], entry.fields["proto"])
}

type noContentRoundTripper struct{}

func (*noContentRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusNoContent,
		Body:       http.NoBody,
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type logEntry struct {
	level  string
	msg    string
	fields map[string]interface{}
}

type recordingLogger struct {
	entries []logEntry
}

func (l *recordingLogger) Info(msg string, ctx ...interface{}) {
	l.entries = append(l.entries, logEntry{level: "info", msg: msg, fields: logFields(ctx)})
}

func (l *recordingLogger) Error(msg string, ctx ...interface{}) {
	l.entries = append(l.entries, logEntry{level: "error", msg: msg, fields: logFields(ctx)})
}

func logFields(ctx []interface{}) map[string]interface{} {
	fields := make(map[string]interface{}, len(ctx)/2)
	for i := 0; i+1 < len(ctx); i += 2 {
		key, ok := ctx[i].(string)
		if ok {
			fields[key] = ctx[i+1]
		}
	}
	return fields
}

func newReplayableRequest(t *testing.T, body string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "https://rpc.example.com", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func mustParseURL(t *testing.T, rawURL string) *url.URL {
	t.Helper()
	endpoint, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	return endpoint
}
