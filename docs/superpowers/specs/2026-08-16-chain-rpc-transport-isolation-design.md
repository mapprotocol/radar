# Chain RPC Transport Isolation Design

## Goal

Reduce the blast radius of slow EVM RPC connections and prevent a single RPC
call from blocking a chain sync for the current 60-second client timeout.

The change isolates each EVM chain's HTTP connection pool, applies deadlines
based on RPC cost, and discards that chain's idle connections after a timeout or
network failure. It improves failure containment and recovery; it does not claim
to make a slow upstream RPC server respond faster.

## Scope

The change covers EVM HTTP and HTTPS RPC calls made by the `radar cli`
subcommand:

- `eth_blockNumber` calls used to obtain the latest block;
- regular `eth_getLogs` calls in the sync loop;
- historical `eth_getLogs` calls in range scans; and
- block-header calls used while processing matched logs.

The change does not add endpoint configuration, automatic retries, endpoint
failover, or a per-host connection limit. It does not change write RPC calls or
non-EVM chain clients.

## Transport Ownership

Each EVM `Connection` owns one cloned `http.Transport`. The clone starts from
`http.DefaultTransport` so it retains proxy discovery, HTTP/2 support, dialer
behavior, and other Go defaults while maintaining a connection pool independent
from every other chain.

The owned transport uses these overrides:

- `IdleConnTimeout`: 30 seconds;
- `TLSHandshakeTimeout`: 3 seconds; and
- `ExpectContinueTimeout`: 1 second.

`MaxConnsPerHost` remains zero, which means no explicit total connection limit.
This avoids introducing client-side queuing under overlapping RPC calls.
`ForceAttemptHTTP2` remains enabled through the cloned default transport.

The RPC logging transport continues to wrap the base transport. Its request log
contract and duration measurement remain unchanged.

## Deadlines

The HTTP client has a 30-second whole-request timeout as a final upper bound.
Individual read RPC calls use shorter context deadlines:

| RPC use | Deadline |
| --- | ---: |
| Latest block (`eth_blockNumber`) | 3 seconds |
| Block header | 5 seconds |
| Regular sync logs (`eth_getLogs`) | 10 seconds |
| Historical range-scan logs (`eth_getLogs`) | 30 seconds |

There is no transport-wide `ResponseHeaderTimeout`. A global 10-second value
would contradict the 30-second historical scan deadline. The per-call contexts
cover connection acquisition, request transfer, response-header wait, and
response decoding through the go-ethereum RPC call.

## Connection Recovery

The EVM `Connection` retains its owned transport and exposes a method that
closes only idle connections. When one of the scoped calls returns because its
context deadline expired or because of a network error, the caller closes that
chain's idle connections before continuing existing error handling.

Active requests are not forcibly closed by this cleanup. A subsequent request
may establish a fresh TCP/TLS connection. Closing the EVM connection during
normal shutdown also closes its idle connections.

JSON-RPC application errors do not trigger connection cleanup. HTTP responses
such as 502, 503, and 504 are not retried in this change.

## Watchdog Behavior

The existing watchdog behavior is retained. Its unbuffered stop signal waits
until the current sync loop can receive it, so this change does not add a mutex
or `singleflight`. Shorter RPC contexts reduce the time an in-flight read can
delay that handoff.

## Error Handling

Existing logging, metrics, alarms, sleeps, and retry loops remain in place. The
new deadlines surface as the original RPC call's error and therefore follow the
same paths as current failures. No request is automatically duplicated.

Context cancellation from a parent, where one exists, is preserved. The helper
that applies an RPC deadline must derive from the supplied context rather than
replace it with `context.Background()`.

## Testing

Focused tests will verify:

- two EVM connections receive distinct base transports;
- the transport is cloned from Go defaults and has the selected timeout values;
- the RPC logging client wraps the supplied base transport;
- each RPC category uses its intended deadline;
- a deadline or network error closes idle connections;
- a JSON-RPC application error does not close idle connections; and
- normal connection close also clears owned idle connections.

Existing RPC logging, EVM package, and repository tests will be run after the
implementation.

## Success Criteria

- Different EVM chains no longer share TCP/TLS/HTTP2 connection pools.
- A latest-block request cannot hold a sync iteration for longer than roughly
  three seconds, subject to cancellation scheduling.
- Regular log scans and block-header reads observe their specified bounds.
- Historical scans retain a longer 30-second bound.
- Timeout and network failures cause only the affected chain's idle connections
  to be discarded.
- No new client-side two-connection bottleneck, automatic retry, or endpoint
  configuration is introduced.
