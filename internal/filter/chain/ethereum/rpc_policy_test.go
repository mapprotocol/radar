package ethereum

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

type recordingIdleTransport struct {
	closeCalls int
}

func (*recordingIdleTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("not used")
}

func (t *recordingIdleTransport) CloseIdleConnections() {
	t.closeCalls++
}

type testNetworkError struct{}

func (testNetworkError) Error() string   { return "network failed" }
func (testNetworkError) Timeout() bool   { return false }
func (testNetworkError) Temporary() bool { return true }

func TestRPCTimeoutPolicy(t *testing.T) {
	if latestBlockRPCTimeout != 3*time.Second || blockHeaderRPCTimeout != 5*time.Second ||
		filterLogsRPCTimeout != 10*time.Second || historicalLogsRPCTimeout != 30*time.Second {
		t.Fatal("unexpected RPC timeout policy")
	}
}

func TestCloseIdleConnectionsOnRPCError(t *testing.T) {
	tests := []struct {
		name   string
		ctxErr error
		err    error
		want   int
	}{
		{name: "deadline", ctxErr: context.DeadlineExceeded, err: context.DeadlineExceeded, want: 1},
		{name: "network", err: testNetworkError{}, want: 1},
		{name: "application", err: errors.New("json-rpc error")},
		{name: "success"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := new(recordingIdleTransport)
			connection := &Connection{transport: transport}
			closeIdleConnectionsOnRPCError(connection, tt.ctxErr, tt.err)
			if transport.closeCalls != tt.want {
				t.Fatalf("close calls = %d, want %d", transport.closeCalls, tt.want)
			}
		})
	}
}

func TestConnectionCloseClosesOwnedIdleConnections(t *testing.T) {
	transport := new(recordingIdleTransport)
	connection := &Connection{transport: transport}

	connection.Close()

	if transport.closeCalls != 1 {
		t.Fatalf("close calls = %d, want 1", transport.closeCalls)
	}
}

func TestRunRPCCallUsesDeadlineAndRecoversConnection(t *testing.T) {
	transport := new(recordingIdleTransport)
	connection := &Connection{transport: transport}
	started := time.Now()

	_, err := runRPCCall(context.Background(), 20*time.Millisecond, connection,
		func(ctx context.Context) (string, error) {
			deadline, ok := ctx.Deadline()
			if !ok || deadline.Sub(started) > 50*time.Millisecond {
				t.Fatalf("deadline = %v, want a short deadline", deadline)
			}
			<-ctx.Done()
			return "", ctx.Err()
		})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want %v", err, context.DeadlineExceeded)
	}
	if transport.closeCalls != 1 {
		t.Fatalf("close calls = %d, want 1", transport.closeCalls)
	}
}

func TestRunRPCCallPreservesParentCancellation(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	cancelParent()

	_, err := runRPCCall(parent, time.Minute, nil, func(ctx context.Context) (string, error) {
		return "", ctx.Err()
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want %v", err, context.Canceled)
	}
}
