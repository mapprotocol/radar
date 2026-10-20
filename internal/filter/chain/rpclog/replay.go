package rpclog

import (
	"bytes"
	"io"
	"net/http"
)

// replayable lets net/http retry a json-rpc POST once on a new connection when
// a reused connection is reset before any response byte.
//
// Cloudflare resets established flows when anycast moves them to another pop
// (confirmed by packet capture on 2026-10-20), and net/http only retries such a
// failure for replayable requests. A POST is replayable when it can rewind its
// body and carries an idempotency key. This restores GetBody when the caller
// left it unset and adds an empty X-Idempotency-Key, which net/http honours
// without sending it on the wire. Failures on a fresh connection are still
// returned as is, so a server that really is down is not hidden.
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
