# RPC HTTP Trace Logging Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox syntax for tracking.

**Goal:** Add request-local HTTP phase timings and connection reuse metadata to every chain RPC completion log, then print real values from a local TLS/HTTP2 exchange.

**Architecture:** A mutex-protected `requestTrace` owns one standard `httptrace.ClientTrace` per request. `Transport.RoundTrip` attaches it, delegates to the existing base transport, and appends a snapshot to the existing completion log.

**Tech Stack:** Go `net/http/httptrace`, `crypto/tls`, `sync`, `httptest`, and `testing`.

---

### Task 1: Request Trace Collector

**Files:**
- Create: `internal/filter/chain/rpclog/trace.go`
- Create: `internal/filter/chain/rpclog/trace_test.go`

- [ ] **Step 1: Write the failing deterministic test**

Drive `GetConn/GotConn`, DNS, TCP, TLS, `WroteRequest`, and
`GotFirstResponseByte` callbacks with a sequence clock. Assert:

```go
want := traceTimings{
    ConnWait: 5 * time.Millisecond,
    DNS: 3 * time.Millisecond,
    TCPConnect: 7 * time.Millisecond,
    TLSHandshake: 11 * time.Millisecond,
    ServerWait: 19 * time.Millisecond,
    Reused: true,
    WasIdle: true,
    IdleTime: 2 * time.Second,
}
```

- [ ] **Step 2: Verify RED**

Run `go test ./internal/filter/chain/rpclog -run TestRequestTraceSnapshot -v`.
Expected: build failure because `newRequestTrace` is undefined.

- [ ] **Step 3: Implement the collector**

Create these package-private APIs:

```go
type traceTimings struct {
    ConnWait, DNS, TCPConnect, TLSHandshake, ServerWait time.Duration
    Reused, WasIdle bool
    IdleTime time.Duration
}
func newRequestTrace(now func() time.Time) *requestTrace
func (t *requestTrace) clientTrace() *httptrace.ClientTrace
func (t *requestTrace) withRequest(req *http.Request) *http.Request
func (t *requestTrace) snapshot() traceTimings
```

Protect callbacks with one mutex. Pair TCP callbacks with a
`map[string][]time.Time` keyed by network and address. Missing starts add zero.
`withRequest` must use `httptrace.WithClientTrace`.

- [ ] **Step 4: Verify GREEN and commit**

```bash
gofmt -w internal/filter/chain/rpclog/trace.go internal/filter/chain/rpclog/trace_test.go
go test -race ./internal/filter/chain/rpclog -run TestRequestTraceSnapshot -v
git add internal/filter/chain/rpclog/trace.go internal/filter/chain/rpclog/trace_test.go
git commit -m "feat: collect chain RPC HTTP trace timings"
```

### Task 2: Completion Log Fields

**Files:**
- Modify: `internal/filter/chain/rpclog/transport.go:105-122`
- Modify: `internal/filter/chain/rpclog/transport_test.go:105-220`

- [ ] **Step 1: Write failing assertions**

Require `conn_wait`, `dns`, `tcp_connect`, `tls_handshake`, `server_wait`,
`reused`, `was_idle`, `idle_time`, and `proto` on success and error logs. A
synthetic base without callbacks must produce zero durations and
`proto=unknown` unless `resp.Proto` is set.

- [ ] **Step 2: Verify RED**

Run `go test ./internal/filter/chain/rpclog -run 'TestTransportLogs(CompletedRequest|RequestError)' -v`.
Expected: missing-field assertions fail.

- [ ] **Step 3: Attach and log the collector**

```go
started := t.now()
trace := newRequestTrace(t.now)
resp, err := t.base.RoundTrip(trace.withRequest(req))
duration := t.now().Sub(started)
timings := trace.snapshot()
proto := unknown
if resp != nil && resp.Proto != "" {
    proto = resp.Proto
}
```

Append all nine fields after `duration`, preserving status placement, one log
per request, and original response/error return values.

- [ ] **Step 4: Verify GREEN and commit**

```bash
go test ./internal/filter/chain/rpclog -run 'TestTransportLogs(CompletedRequest|RequestError)' -v
git add internal/filter/chain/rpclog/transport.go internal/filter/chain/rpclog/transport_test.go
git commit -m "feat: log chain RPC HTTP trace phases"
```

### Task 3: Real Local TLS/HTTP2 Log

**Files:**
- Modify: `internal/filter/chain/rpclog/transport_test.go`

- [ ] **Step 1: Write the integration test**

Use `httptest.NewUnstartedServer`, set `EnableHTTP2=true`, and sleep 25ms in
the handler. Send two replayable JSON-RPC POST requests through the production
logging transport; drain and close response one before request two. Assert the
first request uses HTTP/2 with positive TLS/server wait and `reused=false`.
Assert request two has `reused=true`, `was_idle=true`, zero TLS, and positive
server wait. Print both actual entries using `t.Logf` in production field order.

- [ ] **Step 2: Capture real output**

Run `go test ./internal/filter/chain/rpclog -run TestTransportLogsRealHTTPTrace -count=1 -v`.
Expected: pass and print fresh/reused lines with measured local timings.

- [ ] **Step 3: Full verification and commit**

```bash
go test -race ./internal/filter/chain/rpclog
go test ./...
git diff --check
git add internal/filter/chain/rpclog/transport_test.go
git commit -m "test: verify real RPC HTTP trace logging"
```
