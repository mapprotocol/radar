package rpclog

import (
	"crypto/tls"
	"net/http/httptrace"
	"testing"
	"time"
)

func TestRequestTraceSnapshot(t *testing.T) {
	base := time.Unix(100, 0)
	clock := &sequenceClock{times: []time.Time{
		base, base.Add(5 * time.Millisecond),
		base.Add(10 * time.Millisecond), base.Add(13 * time.Millisecond),
		base.Add(20 * time.Millisecond), base.Add(27 * time.Millisecond),
		base.Add(30 * time.Millisecond), base.Add(41 * time.Millisecond),
		base.Add(50 * time.Millisecond), base.Add(69 * time.Millisecond),
	}}
	trace := newRequestTrace(clock.now)
	hooks := trace.clientTrace()

	hooks.GetConn("rpc.example.com")
	hooks.GotConn(httptrace.GotConnInfo{Reused: true, WasIdle: true, IdleTime: 2 * time.Second})
	hooks.DNSStart(httptrace.DNSStartInfo{})
	hooks.DNSDone(httptrace.DNSDoneInfo{})
	hooks.ConnectStart("tcp", "127.0.0.1:443")
	hooks.ConnectDone("tcp", "127.0.0.1:443", nil)
	hooks.TLSHandshakeStart()
	hooks.TLSHandshakeDone(tls.ConnectionState{}, nil)
	hooks.WroteRequest(httptrace.WroteRequestInfo{})
	hooks.GotFirstResponseByte()

	want := traceTimings{
		ConnWait:     5 * time.Millisecond,
		DNS:          3 * time.Millisecond,
		TCPConnect:   7 * time.Millisecond,
		TLSHandshake: 11 * time.Millisecond,
		ServerWait:   19 * time.Millisecond,
		Reused:       true,
		WasIdle:      true,
		IdleTime:     2 * time.Second,
	}
	if got := trace.snapshot(); got != want {
		t.Fatalf("snapshot = %#v, want %#v", got, want)
	}
}

type sequenceClock struct {
	times []time.Time
	index int
}

func (c *sequenceClock) now() time.Time {
	now := c.times[c.index]
	c.index++
	return now
}
