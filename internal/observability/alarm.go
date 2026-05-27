package observability

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// AlarmRule is the alert specification this package supports today.
// We deliberately keep it simple — one threshold over a sustained window
// per (chain, role). If you need richer rules (rate, deltas, etc.) extend
// here rather than introducing a full PromQL evaluator.
type AlarmRule struct {
	Name      string        // human-readable rule name, included in the message
	Threshold int64         // fallback lag threshold in blocks, used when BlockTimes has no entry for the chain
	For       time.Duration // how long the breach must be sustained before firing
	Cooldown  time.Duration // minimum time between two alarms for the same series

	// LagThreshold is the time-based threshold. When BlockTimes has an entry
	// for the chain, the rule fires only if `block_lag * blockTime >
	// LagThreshold`. This is more meaningful than a fixed block count because
	// 50 blocks means very different things on Matic (~2s/block, 100s) vs
	// Arbitrum (~0.25s/block, 12s) vs Ethereum (~12s/block, 10min).
	LagThreshold time.Duration
	// BlockTimes maps lowercased chain name → average seconds per block.
	// Chains not in the map fall back to the block-count Threshold above.
	BlockTimes map[string]float64
}

// DefaultBlockTimes is a best-effort table of average seconds per block for
// the chains we commonly observe. Tune per deployment if needed.
var DefaultBlockTimes = map[string]float64{
	"ethereum": 12,
	"eth":      12,
	"bsc":      3,
	"bnb":      3,
	"matic":    2,
	"polygon":  2,
	"arbitrum": 0.25,
	"arb":      0.25,
	"optimism": 2,
	"op":       2,
	"base":     2,
	"linea":    12,
	"merlin":   6,
	"tron":     3,
	"ton":      5,
	"xrp":      4,
	"near":     1,
	"map":      5,
	"mapchain": 5,
}

// DefaultBlockLagRule: fire when current-vs-latest gap represents more than 5
// minutes of clock-time lag, sustained for 5 minutes, 10-minute cooldown.
// Falls back to a 50-block count threshold for unknown chains.
func DefaultBlockLagRule() AlarmRule {
	return AlarmRule{
		Name:         "block_lag_high",
		Threshold:    50,
		For:          5 * time.Minute,
		Cooldown:     10 * time.Minute,
		LagThreshold: 5 * time.Minute,
		BlockTimes:   DefaultBlockTimes,
	}
}

// timeBehind returns the lag converted to wall-clock duration for `chain` if
// a block time is known. ok=false means fall back to block-count threshold.
func (r AlarmRule) timeBehind(chain string, lag int64) (time.Duration, bool) {
	if r.BlockTimes == nil {
		return 0, false
	}
	bt, ok := r.BlockTimes[strings.ToLower(chain)]
	if !ok || bt <= 0 {
		return 0, false
	}
	return time.Duration(float64(lag) * bt * float64(time.Second)), true
}

// isBreached reports whether the lag exceeds the rule's effective threshold.
// Prefers the time-based check when block time is known; falls back to block
// count for unknown chains.
func (r AlarmRule) isBreached(chain string, lag int64) bool {
	if dur, ok := r.timeBehind(chain, lag); ok {
		return dur > r.LagThreshold
	}
	return lag > r.Threshold
}

// StartBlockLagAlarms launches a goroutine that polls every chain state and
// fires the configured rule via cfg.AlarmFn. The goroutine exits on Stop().
func (o *Observability) StartBlockLagAlarms(rule AlarmRule) {
	if o.cfg.AlarmFn == nil {
		return
	}
	go o.runBlockLagLoop(rule)
}

func (o *Observability) runBlockLagLoop(rule AlarmRule) {
	type seriesState struct {
		breachStart time.Time // zero = not in breach
		lastAlarmed time.Time
	}
	var (
		mu     sync.Mutex
		series = map[string]*seriesState{}
	)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-o.stopCh:
			return
		case <-ticker.C:
		}

		o.mu.RLock()
		snaps := make([]chainSnapshot, 0, len(o.chains))
		for _, cs := range o.chains {
			snaps = append(snaps, cs.snapshot())
		}
		o.mu.RUnlock()

		now := time.Now()
		for _, s := range snaps {
			key := s.Chain + "/" + s.Role
			// Skip chains that have never reported real progress. A registered
			// chain whose sync loop never ran (e.g. RPC failed at startup) has
			// CurrentBlock=0 and LatestBlock=0; we don't want it to trigger a
			// huge fake block_lag alarm. Once either gauge moves, evaluation
			// kicks in normally.
			if s.CurrentBlock == 0 || s.LatestBlock == 0 {
				continue
			}
			mu.Lock()
			st, ok := series[key]
			if !ok {
				st = &seriesState{}
				series[key] = st
			}
			breached := rule.isBreached(s.Chain, s.BlockLag)
			if !breached {
				st.breachStart = time.Time{}
				mu.Unlock()
				continue
			}
			if st.breachStart.IsZero() {
				st.breachStart = now
				mu.Unlock()
				continue
			}
			if now.Sub(st.breachStart) < rule.For {
				mu.Unlock()
				continue
			}
			if !st.lastAlarmed.IsZero() && now.Sub(st.lastAlarmed) < rule.Cooldown {
				mu.Unlock()
				continue
			}
			st.lastAlarmed = now
			breachAge := now.Sub(st.breachStart).Truncate(time.Second)
			mu.Unlock()

			behindStr := "?"
			if dur, ok := rule.timeBehind(s.Chain, s.BlockLag); ok {
				behindStr = "~" + humanDuration(dur)
			}
			msg := fmt.Sprintf(
				"[%s] %s: block_lag=%d (%s behind) above_threshold_for=%s (current=%d latest=%d)",
				rule.Name, key, s.BlockLag,
				behindStr, breachAge, s.CurrentBlock, s.LatestBlock,
			)
			func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				o.cfg.AlarmFn(ctx, msg)
			}()
		}
	}
}

// humanDuration formats a duration as "Nh Nm" / "Nm Ns" / "Ns" — easier to
// scan than Go's default "1h2m3s".
func humanDuration(d time.Duration) string {
	if d < time.Second {
		return "<1s"
	}
	d = d.Truncate(time.Second)
	days := int(d / (24 * time.Hour))
	d -= time.Duration(days) * 24 * time.Hour
	h := int(d / time.Hour)
	d -= time.Duration(h) * time.Hour
	m := int(d / time.Minute)
	d -= time.Duration(m) * time.Minute
	s := int(d / time.Second)
	switch {
	case days > 0:
		return fmt.Sprintf("%dd%dh", days, h)
	case h > 0:
		return fmt.Sprintf("%dh%dm", h, m)
	case m > 0:
		return fmt.Sprintf("%dm%ds", m, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}
