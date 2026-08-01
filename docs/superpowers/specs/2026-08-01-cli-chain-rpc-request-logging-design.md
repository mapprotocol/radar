# CLI Chain RPC Request Logging Design

## Goal

Add one structured completion log for every HTTP request that the `radar cli`
subcommand sends to a chain RPC endpoint configured in `chains[].endpoint`.
The log must identify both the HTTP method and JSON-RPC method, report response
latency, and remove credentials carried in URL user information or query
parameters.

## Scope

The change covers HTTP and HTTPS chain RPC requests made by the EVM and XRP
clients. These are the current clients that send requests to configured chain
endpoints over HTTP.

The change does not cover:

- inbound requests served by the `api` subcommand;
- the observability HTTP server;
- OpsHub reports, alarm webhooks, or Butter requests;
- Near's Redis data flow;
- TON lite-client traffic; or
- WebSocket RPC traffic.

## Approach

Introduce a chain-RPC-specific `http.RoundTripper` and install it on the HTTP
clients constructed for EVM and XRP. Instrumenting the transport keeps request
logging in one place and includes calls made internally by go-ethereum's RPC
client without changing individual RPC call sites.

The transport wraps an existing base transport and remains stateless so it can
be used concurrently. Before delegating a request, it reads a replayable copy
of the JSON request body, extracts its RPC method names, and leaves the original
body untouched. After the base transport returns, it logs the request outcome.

## Log Contract

A successful request emits an info-level structured log with these fields:

- `http_method`: HTTP method such as `POST`;
- `rpc_method`: JSON-RPC method such as `eth_blockNumber`;
- `endpoint`: sanitized endpoint;
- `status`: HTTP status code; and
- `duration`: elapsed time from starting the transport call until response
  headers arrive.

Example:

```text
Chain RPC request completed http_method=POST rpc_method=eth_blockNumber endpoint=https://rpc.example.com/v1 status=200 duration=128ms
```

A transport error emits an error-level log with the same request fields,
`status=0`, and an `err` field. Each attempted HTTP request produces exactly one
completion log.

For a JSON-RPC batch, `rpc_method` contains the request methods in their original
order, joined with commas. If the request body is missing, non-replayable, or
not recognizable JSON-RPC, `rpc_method` is `unknown`. Method extraction is
best-effort and never blocks or changes the outbound request.

## Endpoint Sanitization

The logged endpoint retains only its scheme, hostname with optional port, and
path. User information, query parameters, and fragments are removed. Request
parameters, request headers, and response bodies are never logged.

The path is retained verbatim as requested. A provider token embedded directly
in a path segment cannot be distinguished reliably from an ordinary path and
would therefore remain visible; deployments must not place secrets in the path
when this logging is enabled.

For example:

```text
https://user:secret@rpc.example.com/v1/key?token=secret#fragment
```

is logged as:

```text
https://rpc.example.com/v1/key
```

If URL parsing fails, the logger uses `unknown` instead of logging the raw value.

## Error Handling

Logging must not alter HTTP behavior. The wrapper returns the original response
and error unchanged. Failures while extracting an RPC method or sanitizing the
endpoint affect only their corresponding log fields. Panics are not introduced
or recovered by this component.

The duration measures transport latency through receipt of response headers. It
does not include subsequent response-body decoding.

## Testing

Focused unit tests will verify:

- extraction of a method from a single JSON-RPC request;
- extraction and ordering of methods from a batch request;
- preservation of the request body passed to the wrapped transport;
- stripping of endpoint user information, query parameters, and fragments;
- successful response logging with HTTP status and positive duration;
- transport-error logging with `status=0`; and
- graceful fallback to `rpc_method=unknown` and `endpoint=unknown`.

Integration-focused tests will verify that the EVM and XRP HTTP clients install
the logging transport. The relevant package tests and the repository test suite
will be run after implementation.

## Success Criteria

- Every EVM or XRP HTTP request to `chains[].endpoint` emits exactly one log.
- Logs contain HTTP method, RPC method, sanitized endpoint, status, and duration.
- Failed requests also emit one useful completion log.
- URL user information and query parameters are not logged; RPC parameters,
  headers, and response bodies are also excluded.
- Existing RPC request and response behavior remains unchanged.
