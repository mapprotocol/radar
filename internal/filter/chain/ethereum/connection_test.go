package ethereum

import (
	"testing"
	"time"

	"github.com/mapprotocol/filter/internal/filter/chain/rpclog"
)

func TestNewHTTPClientUsesRPCLoggingTransport(t *testing.T) {
	client := newHTTPClient()
	if client.Timeout != time.Minute {
		t.Fatalf("timeout = %v, want %v", client.Timeout, time.Minute)
	}
	if _, ok := client.Transport.(*rpclog.Transport); !ok {
		t.Fatalf("transport = %T, want *rpclog.Transport", client.Transport)
	}
}
