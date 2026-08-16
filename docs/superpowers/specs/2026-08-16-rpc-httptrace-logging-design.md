# RPC HTTP Trace Logging Design

## Goal

Extend the existing single completion log for each chain RPC HTTP request with
timings that identify whether latency was spent obtaining a connection,
resolving DNS, connecting TCP, negotiating TLS, or waiting for the first
response byte.

The trace is diagnostic instrumentation. It must not change request behavior,
retry requests, or increase the number of production log entries per request.

## Approach

Attach a standard-library `httptrace.ClientTrace` to the request context inside
the RPC logging transport. The trace records request-local state while the
existing base transport performs the request. After `RoundTrip` returns, append
a snapshot of the trace state to the existing completion log.

This keeps one completion log per attempted request. Separate log lines for
each trace callback were rejected because concurrent chains would produce noisy
and difficult-to-correlate output. Metrics-only instrumentation was rejected
because it cannot explain one specific slow request.

If the caller already installed an `httptrace.ClientTrace`, attaching the RPC
trace through `httptrace.WithClientTrace` preserves the existing hooks.

## Log Contract

The existing fields remain unchanged:

- `http_method`
- `rpc_method`
- `endpoint`
- `status`
- `duration`
- `err` on transport errors

Every completion log adds:

- `conn_wait`: elapsed time from `GetConn` until `GotConn`;
- `dns`: accumulated time between `DNSStart` and `DNSDone`;
- `tcp_connect`: accumulated time between matching `ConnectStart` and
  `ConnectDone` callbacks;
- `tls_handshake`: elapsed time from `TLSHandshakeStart` to
  `TLSHandshakeDone`;
- `server_wait`: elapsed time from `WroteRequest` to
  `GotFirstResponseByte`;
- `reused`: whether the selected connection was previously used;
- `was_idle`: whether the selected connection came from the idle pool;
- `idle_time`: how long the selected connection had been idle; and
- `proto`: the response protocol, such as `HTTP/1.1` or `HTTP/2.0`.

`duration` remains the authoritative total duration from entering the logging
transport until the base transport returns response headers or an error.

## Missing Events

HTTP transports do not emit every trace phase for every request. A reused
connection normally has no DNS, TCP-connect, or TLS-handshake callbacks. Missing
duration phases are logged as `0s`, missing booleans are `false`, and a missing
response protocol is logged as `unknown`.

The fields represent observed client events, not a mathematical partition of
`duration`. Callback scheduling and unobserved work mean their sum is not
required to equal the total.

## Concurrency

Trace callbacks may be invoked from different goroutines. Each request owns one
trace state protected by a mutex. TCP connect attempts are paired by network and
address so concurrent IPv4/IPv6 attempts do not overwrite one another.

The trace snapshot is taken only after the base transport returns. No trace
state is shared across requests or chains.

## Error Handling

Trace collection is best effort. Missing callbacks and trace callback errors do
not alter the response or returned error. Existing URL sanitization and JSON-RPC
method extraction remain unchanged.

For a transport error, phases observed before the error are logged, `status`
remains zero, and `proto` is `unknown` when no response exists.

## Testing

Unit tests will drive the trace callbacks directly with a deterministic clock
and verify all field durations, connection flags, protocol, and error-path
zero values.

An integration test will send real requests through the production logging
transport to a local TLS server with HTTP/2 enabled. The handler will delay its
response so `server_wait` is measurable. A second request will verify connection
reuse. The verbose test output will print one actual structured completion line
whose values came from that local network exchange.

Existing RPC logging tests and the repository test suite will be run after the
change.

## Success Criteria

- Every current chain RPC completion log contains all trace fields.
- A new connection reports non-zero observed setup phases where the local
  runtime emits them.
- A reused connection reports `reused=true` and does not invent DNS, TCP, or TLS
  time.
- A delayed local handler produces a non-zero `server_wait`.
- The total log count, request body, response, and error behavior remain
  unchanged.
- A real local integration-test log is available for inspection.
