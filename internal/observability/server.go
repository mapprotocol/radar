package observability

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/pprof"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Config holds the public surface of an Observability instance.
type Config struct {
	// Addr is the listen address for the HTTP server. Empty disables it
	// (everything else still works in-process).
	Addr string
	// Namespace is the Prometheus metric prefix (e.g. "radar", "compass").
	Namespace string
	// AlarmFn is invoked when the rule engine fires. nil disables alarms.
	AlarmFn func(ctx context.Context, msg string)
}

// Observability wraps Metrics + /status + pprof + alarm rules behind a
// single HTTP server that the main binary embeds.
type Observability struct {
	Metrics *Metrics
	cfg     Config

	mu     sync.RWMutex
	chains map[string]*ChainState // key: chain+role

	server  *http.Server
	stopCh  chan struct{}
	stopped bool
}

// New constructs an Observability instance with a fresh registry.
func New(namespace string, cfg Config) *Observability {
	if cfg.Namespace == "" {
		cfg.Namespace = namespace
	}
	return &Observability{
		Metrics: newMetrics(cfg.Namespace),
		cfg:     cfg,
		chains:  make(map[string]*ChainState),
		stopCh:  make(chan struct{}),
	}
}

// RegisterChain creates (or returns the existing) ChainState for chain+role.
func (o *Observability) RegisterChain(chain, role string) *ChainState {
	key := chain + "/" + role
	o.mu.Lock()
	defer o.mu.Unlock()
	if existing, ok := o.chains[key]; ok {
		return existing
	}
	cs := newChainState(o.Metrics, chain, role, nowUnix())
	o.chains[key] = cs
	return cs
}

// StartHTTP starts the embedded HTTP server.
// Returns immediately; the server runs in its own goroutine.
// Routes:
//
//	/metrics            Prometheus exposition
//	/status             JSON snapshot of all chain states
//	/dashboard          Human-friendly chain health dashboard
//	/healthz            Liveness (200 OK if process is up)
//	/debug/pprof/*      Go runtime profiling endpoints
func (o *Observability) StartHTTP() {
	if o.cfg.Addr == "" {
		return
	}
	mux := http.NewServeMux()
	visualLimiter := newIPRateLimiter(5, time.Minute)
	mux.HandleFunc("/", visualLimiter.wrap(o.handleIndex))
	mux.HandleFunc("/dashboard", visualLimiter.wrap(o.handleDashboard))
	mux.Handle("/metrics", promhttp.HandlerFor(o.Metrics.Registry(), promhttp.HandlerOpts{}))
	mux.HandleFunc("/status", visualLimiter.wrap(o.handleStatus))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	o.server = &http.Server{
		Addr:              o.cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		_ = o.server.ListenAndServe()
	}()
}

// Stop shuts the HTTP server down with a short grace period.
func (o *Observability) Stop() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.stopped {
		return
	}
	o.stopped = true
	close(o.stopCh)
	if o.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = o.server.Shutdown(ctx)
	}
}

// chainSnapshot is the JSON shape rendered by /status. Plain types so the
// reader (jq, curl) doesn't need any tooling beyond stdlib JSON.
type chainSnapshot struct {
	Chain                string `json:"chain"`
	Role                 string `json:"role"`
	CurrentBlock         int64  `json:"current_block"`
	LatestBlock          int64  `json:"latest_block"`
	BlockLag             int64  `json:"block_lag"`
	EstimatedLagSeconds  int64  `json:"estimated_lag_seconds,omitempty"`
	LastProgressTs       int64  `json:"last_progress_unixtime"`
	ProgressStaleSeconds int64  `json:"progress_stale_seconds,omitempty"`
	LastError            string `json:"last_error,omitempty"`
	LastErrorTs          int64  `json:"last_error_unixtime,omitempty"`
	LastErrorAgeSeconds  int64  `json:"last_error_age_seconds,omitempty"`
	StartedTs            int64  `json:"started_unixtime"`
	Health               string `json:"health"`
	HealthReason         string `json:"health_reason"`
}

type statusEnvelope struct {
	UptimeSeconds int64           `json:"uptime_seconds"`
	NowUnixtime   int64           `json:"now_unixtime"`
	Chains        []chainSnapshot `json:"chains"`
}

var startedAt = nowUnix()

func (o *Observability) handleStatus(w http.ResponseWriter, _ *http.Request) {
	now := nowUnix()
	snap := statusEnvelope{
		UptimeSeconds: now - startedAt,
		NowUnixtime:   now,
	}
	o.mu.RLock()
	for _, cs := range o.chains {
		snap.Chains = append(snap.Chains, decorateSnapshot(cs.snapshot(), now))
	}
	o.mu.RUnlock()
	sort.Slice(snap.Chains, func(i, j int) bool {
		if snap.Chains[i].Health != snap.Chains[j].Health {
			return healthRank(snap.Chains[i].Health) < healthRank(snap.Chains[j].Health)
		}
		if snap.Chains[i].Chain != snap.Chains[j].Chain {
			return snap.Chains[i].Chain < snap.Chains[j].Chain
		}
		return snap.Chains[i].Role < snap.Chains[j].Role
	})

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(snap)
}

func (o *Observability) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

func (o *Observability) handleDashboard(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(dashboardHTML))
}

type ipRateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	clients map[string]*ipRateState
	now     func() time.Time
}

type ipRateState struct {
	windowStart time.Time
	count       int
	lastSeen    time.Time
}

func newIPRateLimiter(limit int, window time.Duration) *ipRateLimiter {
	return &ipRateLimiter{
		limit:   limit,
		window:  window,
		clients: make(map[string]*ipRateState),
		now:     time.Now,
	}
}

func (l *ipRateLimiter) wrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		allowed, retryAfter := l.allow(clientIP(r))
		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int((retryAfter+time.Second-1)/time.Second)))
			http.Error(w, "rate limit exceeded: maximum 5 requests per minute per IP", http.StatusTooManyRequests)
			return
		}
		next(w, r)
	}
}

func (l *ipRateLimiter) allow(ip string) (bool, time.Duration) {
	if ip == "" {
		ip = "unknown"
	}
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()

	for key, state := range l.clients {
		if now.Sub(state.lastSeen) > 2*l.window {
			delete(l.clients, key)
		}
	}

	state, ok := l.clients[ip]
	if !ok || now.Sub(state.windowStart) >= l.window {
		l.clients[ip] = &ipRateState{windowStart: now, count: 1, lastSeen: now}
		return true, 0
	}
	state.lastSeen = now
	if state.count >= l.limit {
		return false, state.windowStart.Add(l.window).Sub(now)
	}
	state.count++
	return true, 0
}

func clientIP(r *http.Request) string {
	if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
		return ip
	}
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		if ip := strings.TrimSpace(strings.Split(forwarded, ",")[0]); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func decorateSnapshot(s chainSnapshot, now int64) chainSnapshot {
	if s.LastProgressTs > 0 {
		s.ProgressStaleSeconds = now - s.LastProgressTs
	}
	if s.LastErrorTs > 0 {
		s.LastErrorAgeSeconds = now - s.LastErrorTs
	}
	if d, ok := DefaultBlockLagRule().timeBehind(s.Chain, s.BlockLag); ok {
		s.EstimatedLagSeconds = int64(d.Seconds())
	}
	s.Health, s.HealthReason = deriveHealth(s)
	return s
}

func deriveHealth(s chainSnapshot) (string, string) {
	if s.CurrentBlock == 0 || s.LatestBlock == 0 {
		return "warming", "waiting for first chain progress sample"
	}
	if s.LastErrorAgeSeconds > 0 && s.LastErrorAgeSeconds <= int64(10*time.Minute/time.Second) {
		return "error", "recent error recorded"
	}
	if s.BlockLag > 0 && s.ProgressStaleSeconds > int64(10*time.Minute/time.Second) {
		return "stalled", "block progress has not moved recently"
	}
	rule := DefaultBlockLagRule()
	if rule.isBreached(s.Chain, s.BlockLag) {
		if s.EstimatedLagSeconds > 0 {
			return "lagging", "estimated lag is above threshold"
		}
		return "lagging", "block lag is above threshold"
	}
	return "healthy", "processing is current"
}

func healthRank(status string) int {
	switch status {
	case "error":
		return 0
	case "stalled":
		return 1
	case "lagging":
		return 2
	case "warming":
		return 3
	default:
		return 4
	}
}

// nowUnix is a seam for tests.
var nowUnix = func() int64 { return time.Now().Unix() }

const dashboardHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Radar Observability</title>
  <style>
    :root {
      color-scheme: light dark;
      --bg: #f6f7f9;
      --panel: #ffffff;
      --text: #18202a;
      --muted: #66707c;
      --line: #d9dee5;
      --ok: #12805c;
      --warn: #b7791f;
      --bad: #c53030;
      --info: #2764b5;
      --shadow: 0 8px 24px rgba(22, 31, 44, .08);
    }
    @media (prefers-color-scheme: dark) {
      :root {
        --bg: #111418;
        --panel: #191f26;
        --text: #eef2f6;
        --muted: #9ca7b4;
        --line: #303844;
        --shadow: 0 8px 24px rgba(0, 0, 0, .25);
      }
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      background: var(--bg);
      color: var(--text);
    }
    main { width: min(1180px, calc(100vw - 32px)); margin: 28px auto; }
    header { display: flex; justify-content: space-between; gap: 20px; align-items: flex-end; margin-bottom: 18px; }
    h1 { margin: 0 0 6px; font-size: 28px; font-weight: 720; letter-spacing: 0; }
    .sub, .updated { color: var(--muted); font-size: 14px; }
    .summary { display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); gap: 12px; margin-bottom: 16px; }
    .tile, .panel { background: var(--panel); border: 1px solid var(--line); border-radius: 8px; box-shadow: var(--shadow); }
    .tile { padding: 14px; min-height: 76px; }
    .tile strong { display: block; font-size: 24px; line-height: 1.1; }
    .tile span { display: block; color: var(--muted); font-size: 13px; margin-top: 6px; }
    .panel { overflow-x: auto; }
    table { width: 100%; border-collapse: collapse; min-width: 900px; }
    th, td { padding: 13px 14px; text-align: left; border-bottom: 1px solid var(--line); font-size: 14px; vertical-align: middle; }
    th { color: var(--muted); font-weight: 650; background: color-mix(in srgb, var(--panel), var(--bg) 42%); }
    tr:last-child td { border-bottom: 0; }
    .chain { font-weight: 700; }
    .role { color: var(--muted); font-size: 12px; margin-top: 2px; }
    .pill { display: inline-flex; align-items: center; min-width: 78px; justify-content: center; border-radius: 999px; padding: 5px 10px; font-size: 12px; font-weight: 760; color: #fff; text-transform: uppercase; }
    .healthy { background: var(--ok); }
    .warming { background: var(--info); }
    .lagging { background: var(--warn); }
    .stalled, .error { background: var(--bad); }
    .number { font-variant-numeric: tabular-nums; }
    .reason { max-width: 260px; color: var(--muted); white-space: normal; }
    .empty, .failed { padding: 28px; color: var(--muted); }
    a { color: inherit; }
    @media (max-width: 760px) {
      main { width: min(100vw - 20px, 1180px); margin-top: 18px; }
      header { display: block; }
      .summary { grid-template-columns: repeat(2, minmax(0, 1fr)); }
      h1 { font-size: 24px; }
    }
  </style>
</head>
<body>
  <main>
    <header>
      <div>
        <h1>Radar Observability</h1>
        <div class="sub">Chain sync health from <a href="/status">/status</a> and raw Prometheus data at <a href="/metrics">/metrics</a>.</div>
      </div>
      <div class="updated" id="updated">Loading...</div>
    </header>
    <section class="summary" id="summary"></section>
    <section class="panel" id="panel">
      <div class="empty">Waiting for chain status...</div>
    </section>
  </main>
  <script>
    const order = ["error", "stalled", "lagging", "warming", "healthy"];
    const labels = { error: "Error", stalled: "Stalled", lagging: "Lagging", warming: "Warming", healthy: "Healthy" };
    const fmt = new Intl.NumberFormat();

    function duration(seconds) {
      if (!seconds || seconds < 1) return "-";
      const d = Math.floor(seconds / 86400);
      const h = Math.floor(seconds % 86400 / 3600);
      const m = Math.floor(seconds % 3600 / 60);
      const s = Math.floor(seconds % 60);
      if (d) return d + "d " + h + "h";
      if (h) return h + "h " + m + "m";
      if (m) return m + "m " + s + "s";
      return s + "s";
    }

    function since(ts, now) {
      return ts ? duration(Math.max(0, now - ts)) : "-";
    }

    function renderSummary(chains) {
      const counts = Object.fromEntries(order.map(k => [k, 0]));
      for (const c of chains) counts[c.health] = (counts[c.health] || 0) + 1;
      document.getElementById("summary").innerHTML = order.map(k =>
        '<div class="tile">' +
          '<strong>' + (counts[k] || 0) + '</strong>' +
          '<span>' + labels[k] + '</span>' +
        '</div>'
      ).join("");
    }

    function renderTable(data) {
      const chains = data.chains || [];
      renderSummary(chains);
      document.getElementById("updated").textContent = "Updated " + new Date().toLocaleTimeString() + " - uptime " + duration(data.uptime_seconds);
      if (!chains.length) {
        document.getElementById("panel").innerHTML = '<div class="empty">No chains registered yet.</div>';
        return;
      }
      const rows = chains.map(c => {
        const lastError = c.last_error ? since(c.last_error_unixtime, data.now_unixtime) + " ago" : "-";
        return '<tr>' +
          '<td><div class="chain">' + c.chain + '</div><div class="role">' + c.role + '</div></td>' +
          '<td><span class="pill ' + c.health + '">' + (labels[c.health] || c.health) + '</span></td>' +
          '<td class="number">' + fmt.format(c.current_block || 0) + '</td>' +
          '<td class="number">' + fmt.format(c.latest_block || 0) + '</td>' +
          '<td class="number">' + fmt.format(c.block_lag || 0) + '</td>' +
          '<td>' + duration(c.estimated_lag_seconds) + '</td>' +
          '<td>' + since(c.last_progress_unixtime, data.now_unixtime) + '</td>' +
          '<td>' + lastError + '</td>' +
          '<td class="reason">' + (c.last_error || c.health_reason || "-") + '</td>' +
        '</tr>';
      }).join("");
      document.getElementById("panel").innerHTML =
        '<table>' +
          '<thead>' +
            '<tr>' +
              '<th>Chain</th>' +
              '<th>Health</th>' +
              '<th>Current</th>' +
              '<th>Latest</th>' +
              '<th>Lag</th>' +
              '<th>Behind</th>' +
              '<th>Last Progress</th>' +
              '<th>Last Error</th>' +
              '<th>Reason</th>' +
            '</tr>' +
          '</thead>' +
          '<tbody>' + rows + '</tbody>' +
        '</table>';
    }

    async function refresh() {
      try {
        const res = await fetch("/status", { cache: "no-store" });
        if (!res.ok) throw new Error("HTTP " + res.status);
        renderTable(await res.json());
      } catch (err) {
        document.getElementById("updated").textContent = "Refresh failed: " + err.message;
        document.getElementById("panel").innerHTML = '<div class="failed">Could not load /status.</div>';
      }
    }

    refresh();
    setInterval(refresh, 20000);
  </script>
</body>
</html>`
