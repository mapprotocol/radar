package ethereum

import (
	"net/http"
	"testing"
	"time"

	"github.com/mapprotocol/filter/internal/filter/chain/rpclog"
)

func TestNewHTTPClientUsesRPCLoggingTransport(t *testing.T) {
	client := newHTTPClient(newRPCTransport())
	if client.Timeout != 30*time.Second {
		t.Fatalf("timeout = %v, want %v", client.Timeout, 30*time.Second)
	}
	if _, ok := client.Transport.(*rpclog.Transport); !ok {
		t.Fatalf("transport = %T, want *rpclog.Transport", client.Transport)
	}
}

func TestNewRPCTransportClonesDefaultsForOneChain(t *testing.T) {
	first := newRPCTransport()
	second := newRPCTransport()

	if first == second || first == http.DefaultTransport {
		t.Fatal("each chain must own a distinct transport clone")
	}
	if first.IdleConnTimeout != 30*time.Second {
		t.Fatalf("IdleConnTimeout = %v, want %v", first.IdleConnTimeout, 30*time.Second)
	}
	if first.TLSHandshakeTimeout != 3*time.Second {
		t.Fatalf("TLSHandshakeTimeout = %v, want %v", first.TLSHandshakeTimeout, 3*time.Second)
	}
	if first.ExpectContinueTimeout != time.Second {
		t.Fatalf("ExpectContinueTimeout = %v, want %v", first.ExpectContinueTimeout, time.Second)
	}
	if !first.ForceAttemptHTTP2 {
		t.Fatal("ForceAttemptHTTP2 = false, want true")
	}
	if first.MaxConnsPerHost != 0 {
		t.Fatalf("MaxConnsPerHost = %d, want no explicit limit", first.MaxConnsPerHost)
	}
}
