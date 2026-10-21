package ethereum

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

type connKey struct{}

// TestRPCClientSurvivesH2ConnectionReset runs the production client stack
// (newRPCTransport, newHTTPClient, go-ethereum rpc, ethclient) against an
// HTTP/2 TLS server that resets the TCP connection on the second request, the
// way cloudflare pops reset flows anycast moved to them.
func TestRPCClientSurvivesH2ConnectionReset(t *testing.T) {
	var requests int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = io.Copy(io.Discard, req.Body)
		if atomic.AddInt32(&requests, 1) == 2 {
			raw := req.Context().Value(connKey{}).(*tls.Conn).NetConn().(*net.TCPConn)
			raw.SetLinger(0) // close sends RST
			raw.Close()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":"0x10"}`)
	}))
	server.EnableHTTP2 = true
	server.Config.ConnContext = func(ctx context.Context, c net.Conn) context.Context {
		return context.WithValue(ctx, connKey{}, c)
	}
	server.StartTLS()
	defer server.Close()

	transport := newRPCTransport()
	transport.TLSClientConfig = server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	defer transport.CloseIdleConnections()
	rpcClient, err := rpc.DialOptions(context.Background(), server.URL, rpc.WithHTTPClient(newHTTPClient(transport)))
	if err != nil {
		t.Fatal(err)
	}
	defer rpcClient.Close()
	client := ethclient.NewClient(rpcClient)

	for i := 1; i <= 2; i++ {
		got, err := client.BlockNumber(context.Background())
		if err != nil {
			t.Fatalf("BlockNumber call %d error = %v, want the reset retried", i, err)
		}
		if got != 16 {
			t.Fatalf("BlockNumber call %d = %d, want 16", i, got)
		}
	}
	if got := atomic.LoadInt32(&requests); got != 3 {
		t.Fatalf("server requests = %d, want 3 (ok, reset, retry)", got)
	}
}
