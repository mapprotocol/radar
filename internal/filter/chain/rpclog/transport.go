package rpclog

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const unknown = "unknown"

type rpcRequest struct {
	Method string `json:"method"`
}

func rpcMethods(req *http.Request) string {
	if req == nil || req.GetBody == nil {
		return unknown
	}

	body, err := req.GetBody()
	if err != nil {
		return unknown
	}
	defer body.Close()

	payload, err := io.ReadAll(body)
	if err != nil || len(payload) == 0 {
		return unknown
	}

	var raw json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil || len(raw) == 0 {
		return unknown
	}

	switch raw[0] {
	case '{':
		var request rpcRequest
		if err := json.Unmarshal(raw, &request); err != nil || request.Method == "" {
			return unknown
		}
		return request.Method
	case '[':
		var requests []rpcRequest
		if err := json.Unmarshal(raw, &requests); err != nil || len(requests) == 0 {
			return unknown
		}

		methods := make([]string, len(requests))
		for i, request := range requests {
			if request.Method == "" {
				return unknown
			}
			methods[i] = request.Method
		}
		return strings.Join(methods, ",")
	default:
		return unknown
	}
}

func sanitizeEndpoint(endpoint *url.URL) string {
	if endpoint == nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return unknown
	}

	return (&url.URL{
		Scheme:  endpoint.Scheme,
		Host:    endpoint.Host,
		Path:    endpoint.Path,
		RawPath: endpoint.RawPath,
	}).String()
}

type Logger interface {
	Info(msg string, ctx ...interface{})
	Error(msg string, ctx ...interface{})
}

type Transport struct {
	base   http.RoundTripper
	logger Logger
	now    func() time.Time
}

func NewTransport(base http.RoundTripper, logger Logger) *Transport {
	if base == nil {
		base = http.DefaultTransport
	}
	return &Transport{base: base, logger: logger, now: time.Now}
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	started := t.now()
	resp, err := t.base.RoundTrip(req)
	duration := t.now().Sub(started)
	fields := []interface{}{
		"http_method", req.Method,
		"rpc_method", rpcMethods(req),
		"endpoint", sanitizeEndpoint(req.URL),
		"status", 0,
		"duration", duration,
	}
	if err != nil {
		t.logger.Error("Chain RPC request completed", append(fields, "err", err)...)
		return resp, err
	}
	fields[7] = resp.StatusCode
	t.logger.Info("Chain RPC request completed", fields...)
	return resp, nil
}
