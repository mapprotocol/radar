# Chain RPC Transport Isolation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give every EVM chain an independent HTTP connection pool, bound read RPCs by operation-specific deadlines, and discard only the affected chain's idle connections after timeout or network failure.

**Architecture:** `rpclog.NewHTTPClient` will wrap an explicitly supplied base transport. Each EVM `Connection` will clone and retain its own configured `http.Transport`; focused policy helpers will create RPC contexts and decide when to recycle idle connections. Existing sync error handling remains unchanged after the policy runs.

**Tech Stack:** Go 1.23+, standard `net/http`, `context`, and `net` packages, go-ethereum RPC/ethclient, Go `testing`.

---

### Task 1: Inject the Logging Client's Base Transport

**Files:**
- Modify: `internal/filter/chain/rpclog/transport.go:98`
- Modify: `internal/filter/chain/rpclog/transport_test.go`
- Modify: `internal/filter/chain/xrp/connection.go:40`

- [ ] **Step 1: Write the failing test**

Add to `transport_test.go`:

```go
func TestNewHTTPClientWrapsSuppliedTransport(t *testing.T) {
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Header: make(http.Header), Request: req}, nil
	})
	client := NewHTTPClient(7*time.Second, base)
	if client.Timeout != 7*time.Second {
		t.Fatalf("timeout = %v, want %v", client.Timeout, 7*time.Second)
	}
	transport, ok := client.Transport.(*Transport)
	if !ok || transport.base != base {
		t.Fatalf("transport = %T, want logging wrapper around supplied base", client.Transport)
	}
}
```

- [ ] **Step 2: Verify RED**

Run `go test ./internal/filter/chain/rpclog -run TestNewHTTPClientWrapsSuppliedTransport -v`.
Expected: build failure because `NewHTTPClient` accepts one argument.

- [ ] **Step 3: Implement the API and update XRP**

```go
func NewHTTPClient(timeout time.Duration, base http.RoundTripper) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: NewTransport(base, log.Root()),
	}
}
```

Keep XRP behavior unchanged:

```go
func newHTTPClient() *http.Client {
	return rpclog.NewHTTPClient(time.Minute, http.DefaultTransport)
}
```

- [ ] **Step 4: Verify GREEN**

Run `go test ./internal/filter/chain/rpclog ./internal/filter/chain/xrp -v`.
Expected: both packages pass.

- [ ] **Step 5: Commit**

```bash
git add internal/filter/chain/rpclog/transport.go internal/filter/chain/rpclog/transport_test.go internal/filter/chain/xrp/connection.go
git commit -m "refactor: inject chain RPC base transport"
```

### Task 2: Own One Transport per EVM Connection

**Files:**
- Modify: `internal/filter/chain/ethereum/connection.go:27-64`
- Modify: `internal/filter/chain/ethereum/connection_test.go`

- [ ] **Step 1: Write failing ownership tests**

```go
package ethereum

import (
	"net/http"
	"testing"
	"time"

	"github.com/mapprotocol/filter/internal/filter/chain/rpclog"
)

func TestNewRPCTransportClonesDefaultsForOneChain(t *testing.T) {
	first, second := newRPCTransport(), newRPCTransport()
	if first == second || first == http.DefaultTransport {
		t.Fatal("each chain must own a distinct transport clone")
	}
	if first.IdleConnTimeout != 30*time.Second || first.TLSHandshakeTimeout != 3*time.Second {
		t.Fatalf("transport timeouts = (%v, %v)", first.IdleConnTimeout, first.TLSHandshakeTimeout)
	}
	if first.ExpectContinueTimeout != time.Second || !first.ForceAttemptHTTP2 {
		t.Fatalf("transport protocol settings = (%v, %v)", first.ExpectContinueTimeout, first.ForceAttemptHTTP2)
	}
	if first.MaxConnsPerHost != 0 {
		t.Fatalf("MaxConnsPerHost = %d, want no explicit limit", first.MaxConnsPerHost)
	}
}

func TestNewHTTPClientUsesOwnedTransport(t *testing.T) {
	client := newHTTPClient(newRPCTransport())
	if client.Timeout != 30*time.Second {
		t.Fatalf("timeout = %v, want %v", client.Timeout, 30*time.Second)
	}
	if _, ok := client.Transport.(*rpclog.Transport); !ok {
		t.Fatalf("transport = %T, want *rpclog.Transport", client.Transport)
	}
}
```

- [ ] **Step 2: Verify RED**

Run `go test ./internal/filter/chain/ethereum -run 'TestNew(RPCTransport|HTTPClient)' -v`.
Expected: build failure because `newRPCTransport` and the new client signature do not exist.

- [ ] **Step 3: Implement transport construction and ownership**

```go
const rpcHTTPClientTimeout = 30 * time.Second

type idleRoundTripper interface {
	http.RoundTripper
	CloseIdleConnections()
}

func newRPCTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.IdleConnTimeout = 30 * time.Second
	transport.TLSHandshakeTimeout = 3 * time.Second
	transport.ExpectContinueTimeout = time.Second
	transport.MaxConnsPerHost = 0
	return transport
}

func newHTTPClient(base http.RoundTripper) *http.Client {
	return rpclog.NewHTTPClient(rpcHTTPClientTimeout, base)
}
```

Add `transport idleRoundTripper` to `Connection`. In `Connect`, create the
transport before the HTTP client, close its idle connections if dialing fails,
and assign it to `c.transport` after a successful dial:

```go
transport := newRPCTransport()
cli := newHTTPClient(transport)
withClient := rpc.WithHTTPClient(cli)
rpcClient, err = rpc.DialOptions(context.Background(), c.endpoint, withClient)
if err != nil {
	transport.CloseIdleConnections()
	return err
}
c.transport = transport
```

- [ ] **Step 4: Verify GREEN**

Run `go test ./internal/filter/chain/ethereum -v`.
Expected: package passes.

- [ ] **Step 5: Commit**

```bash
git add internal/filter/chain/ethereum/connection.go internal/filter/chain/ethereum/connection_test.go
git commit -m "feat: isolate EVM RPC transports by chain"
```

### Task 3: Add Deadline and Recovery Policy

**Files:**
- Create: `internal/filter/chain/ethereum/rpc_policy.go`
- Create: `internal/filter/chain/ethereum/rpc_policy_test.go`
- Modify: `internal/filter/chain/ethereum/connection.go:116`

- [ ] **Step 1: Write failing policy tests**

```go
package ethereum

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

type recordingIdleTransport struct{ closeCalls int }

func (*recordingIdleTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("not used")
}
func (t *recordingIdleTransport) CloseIdleConnections() { t.closeCalls++ }

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
		name string
		ctxErr, err error
		want int
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
```

- [ ] **Step 2: Verify RED**

Run `go test ./internal/filter/chain/ethereum -run 'Test(RPCTimeoutPolicy|CloseIdleConnectionsOnRPCError|ConnectionCloseClosesOwnedIdleConnections)' -v`.
Expected: build failure because policy declarations are missing.

- [ ] **Step 3: Implement the policy**

Create `rpc_policy.go`:

```go
package ethereum

import (
	"context"
	"errors"
	"net"
	"time"
)

const (
	latestBlockRPCTimeout = 3 * time.Second
	blockHeaderRPCTimeout = 5 * time.Second
	filterLogsRPCTimeout = 10 * time.Second
	historicalLogsRPCTimeout = 30 * time.Second
)

type idleConnectionCloser interface{ CloseIdleConnections() }

func closeIdleConnectionsOnRPCError(connection interface{}, ctxErr, err error) {
	if err == nil {
		return
	}
	var networkError net.Error
	if !errors.Is(ctxErr, context.DeadlineExceeded) &&
		!errors.Is(err, context.DeadlineExceeded) &&
		!errors.As(err, &networkError) {
		return
	}
	if closer, ok := connection.(idleConnectionCloser); ok {
		closer.CloseIdleConnections()
	}
}
```

Add `Connection.CloseIdleConnections` and call it from `Connection.Close`:

```go
func (c *Connection) CloseIdleConnections() {
	if c.transport != nil {
		c.transport.CloseIdleConnections()
	}
}

func (c *Connection) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
	c.CloseIdleConnections()
}
```

- [ ] **Step 4: Verify GREEN**

Run `go test ./internal/filter/chain/ethereum -v`.
Expected: package passes.

- [ ] **Step 5: Commit**

```bash
git add internal/filter/chain/ethereum/connection.go internal/filter/chain/ethereum/rpc_policy.go internal/filter/chain/ethereum/rpc_policy_test.go
git commit -m "feat: add EVM RPC timeout recovery policy"
```

### Task 4: Run Bounded RPC Calls and Apply Them to EVM Reads

**Files:**
- Modify: `internal/filter/chain/ethereum/connection.go:102-113`
- Modify: `internal/filter/chain/ethereum/sync.go:137-147`
- Modify: `internal/filter/chain/ethereum/sync.go:170-181`
- Modify: `internal/filter/chain/ethereum/sync.go:226-235`
- Modify: `internal/filter/chain/ethereum/rpc_policy_test.go`

- [ ] **Step 1: Add failing bounded-call tests**

```go
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
```

- [ ] **Step 2: Verify RED**

Run `go test ./internal/filter/chain/ethereum -run TestRunRPCCall -v`.
Expected: build failure because `runRPCCall` does not exist.

- [ ] **Step 3: Implement the bounded call helper**

Add to `rpc_policy.go`:

```go
func runRPCCall[T any](
	parent context.Context,
	timeout time.Duration,
	connection interface{},
	call func(context.Context) (T, error),
) (T, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	result, err := call(ctx)
	ctxErr := ctx.Err()
	cancel()
	closeIdleConnectionsOnRPCError(connection, ctxErr, err)
	return result, err
}
```

- [ ] **Step 4: Apply exact deadlines and recovery**

Use `runRPCCall` at each RPC site. Latest block uses:

```go
num, err := runRPCCall(context.Background(), latestBlockRPCTimeout, c,
	func(ctx context.Context) (uint64, error) {
		return c.conn.BlockNumber(ctx)
	})
```

For `rangeScan`, use this complete form:

```go
logs, err := runRPCCall(context.Background(), historicalLogsRPCTimeout, c.conn,
	func(ctx context.Context) ([]types.Log, error) {
		return c.conn.Client().FilterLogs(ctx, ethereum.FilterQuery{
			FromBlock: big.NewInt(i),
			ToBlock:   big.NewInt(i + 20),
			Addresses: []common.Address{common.HexToAddress(event.Address)},
			Topics:    [][]common.Hash{topics},
		})
	})
```

For `mosHandler`, use:

```go
logs, err := runRPCCall(context.Background(), filterLogsRPCTimeout, c.conn,
	func(ctx context.Context) ([]types.Log, error) {
		return c.conn.Client().FilterLogs(ctx, query)
	})
```

For `insert`, use:

```go
header, err := runRPCCall(context.Background(), blockHeaderRPCTimeout, c.conn,
	func(ctx context.Context) (*types.Header, error) {
		return c.conn.Client().HeaderByNumber(ctx,
			big.NewInt(0).SetUint64(inserts[0].ll.BlockNumber))
	})
```

All three chain call sites pass `c.conn`, keeping cleanup scoped to the
affected chain.

- [ ] **Step 5: Format and run focused verification**

```bash
gofmt -w internal/filter/chain/rpclog/transport.go internal/filter/chain/rpclog/transport_test.go internal/filter/chain/ethereum/connection.go internal/filter/chain/ethereum/connection_test.go internal/filter/chain/ethereum/rpc_policy.go internal/filter/chain/ethereum/rpc_policy_test.go internal/filter/chain/ethereum/sync.go internal/filter/chain/xrp/connection.go
go test ./internal/filter/chain/rpclog ./internal/filter/chain/ethereum ./internal/filter/chain/xrp -v
```

Expected: all focused tests pass.

- [ ] **Step 6: Run repository verification**

```bash
go test ./...
git diff --check
```

Expected: all repository tests pass and `git diff --check` prints no output.

- [ ] **Step 7: Commit the completed behavior**

```bash
git add internal/filter/chain/ethereum/connection.go internal/filter/chain/ethereum/sync.go internal/filter/chain/ethereum/rpc_policy_test.go
git commit -m "feat: bound EVM RPC read latency"
```
