package rpclog

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"syscall"
)

// replayable lets net/http retry a json-rpc POST once on a new connection when
// a reused HTTP/1.1 connection is reset before any response byte.
//
// Cloudflare resets established flows when anycast moves them to another pop
// (confirmed by packet capture on 2026-10-20), and net/http only retries such a
// failure for replayable requests. A POST is replayable when it can rewind its
// body and carries an idempotency key. This restores GetBody when the caller
// left it unset and adds an empty X-Idempotency-Key, which net/http honours
// without sending it on the wire.
//
// This covers HTTP/1.1 only. cf-rpc is served over HTTP/2, whose transport
// never retries a bare TCP reset; retryAfterReset covers that case.
func replayable(req *http.Request) (*http.Request, error) {
	out := req.Clone(req.Context())
	if req.Body != nil && req.Body != http.NoBody && req.GetBody == nil {
		body, err := io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, err
		}
		out.Body = io.NopCloser(bytes.NewReader(body))
		out.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		}
	}
	if _, ok := out.Header["X-Idempotency-Key"]; !ok {
		out.Header["X-Idempotency-Key"] = []string{}
	}
	return out, nil
}

// retryAfterReset returns a fresh copy of req to send once more when the
// connection was reset, whatever the HTTP version. A reset connection is
// discarded by the transport, so the retry runs on a new connection. It retries
// only once and only on ECONNRESET, so a server that is really down still
// fails, and never after the caller gave up on the request.
func retryAfterReset(req *http.Request, err error) (*http.Request, bool) {
	if err == nil || !errors.Is(err, syscall.ECONNRESET) || req.Context().Err() != nil {
		return nil, false
	}
	retry := req.Clone(req.Context())
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, false
		}
		retry.Body = body
	} else if req.Body != nil && req.Body != http.NoBody {
		return nil, false
	}
	return retry, true
}
