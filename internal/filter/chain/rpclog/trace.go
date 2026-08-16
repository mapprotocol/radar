package rpclog

import (
	"crypto/tls"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"
)

type traceTimings struct {
	ConnWait     time.Duration
	DNS          time.Duration
	TCPConnect   time.Duration
	TLSHandshake time.Duration
	ServerWait   time.Duration
	Reused       bool
	WasIdle      bool
	IdleTime     time.Duration
}

type requestTrace struct {
	mu            sync.Mutex
	now           func() time.Time
	timings       traceTimings
	connStart     time.Time
	dnsStarts     []time.Time
	connectStarts map[string][]time.Time
	tlsStarts     []time.Time
	wroteRequest  time.Time
}

func newRequestTrace(now func() time.Time) *requestTrace {
	return &requestTrace{
		now:           now,
		connectStarts: make(map[string][]time.Time),
	}
}

func (t *requestTrace) clientTrace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		GetConn: func(string) {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.connStart = t.now()
		},
		GotConn: func(info httptrace.GotConnInfo) {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.timings.ConnWait += elapsed(t.connStart, t.now())
			t.connStart = time.Time{}
			t.timings.Reused = info.Reused
			t.timings.WasIdle = info.WasIdle
			t.timings.IdleTime = info.IdleTime
		},
		DNSStart: func(httptrace.DNSStartInfo) {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.dnsStarts = append(t.dnsStarts, t.now())
		},
		DNSDone: func(httptrace.DNSDoneInfo) {
			t.mu.Lock()
			defer t.mu.Unlock()
			start, remaining := popStart(t.dnsStarts)
			t.dnsStarts = remaining
			t.timings.DNS += elapsed(start, t.now())
		},
		ConnectStart: func(network, addr string) {
			t.mu.Lock()
			defer t.mu.Unlock()
			key := network + "\x00" + addr
			t.connectStarts[key] = append(t.connectStarts[key], t.now())
		},
		ConnectDone: func(network, addr string, _ error) {
			t.mu.Lock()
			defer t.mu.Unlock()
			key := network + "\x00" + addr
			start, remaining := popStart(t.connectStarts[key])
			if len(remaining) == 0 {
				delete(t.connectStarts, key)
			} else {
				t.connectStarts[key] = remaining
			}
			t.timings.TCPConnect += elapsed(start, t.now())
		},
		TLSHandshakeStart: func() {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.tlsStarts = append(t.tlsStarts, t.now())
		},
		TLSHandshakeDone: func(tls.ConnectionState, error) {
			t.mu.Lock()
			defer t.mu.Unlock()
			start, remaining := popStart(t.tlsStarts)
			t.tlsStarts = remaining
			t.timings.TLSHandshake += elapsed(start, t.now())
		},
		WroteRequest: func(httptrace.WroteRequestInfo) {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.wroteRequest = t.now()
		},
		GotFirstResponseByte: func() {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.timings.ServerWait += elapsed(t.wroteRequest, t.now())
			t.wroteRequest = time.Time{}
		},
	}
}

func (t *requestTrace) withRequest(req *http.Request) *http.Request {
	ctx := httptrace.WithClientTrace(req.Context(), t.clientTrace())
	return req.WithContext(ctx)
}

func (t *requestTrace) snapshot() traceTimings {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.timings
}

func popStart(starts []time.Time) (time.Time, []time.Time) {
	if len(starts) == 0 {
		return time.Time{}, starts
	}
	return starts[0], starts[1:]
}

func elapsed(start, end time.Time) time.Duration {
	if start.IsZero() || end.Before(start) {
		return 0
	}
	return end.Sub(start)
}
