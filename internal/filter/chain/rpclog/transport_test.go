package rpclog

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
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
