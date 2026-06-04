package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/log"
)

const defaultOpsHubReportInterval = 15 * time.Second

// OpsHubReporterConfig controls active reporting to Ops Hub's monitoring
// observability endpoint.
type OpsHubReporterConfig struct {
	Endpoint string
	APIKey   string
	Env      string
	Instance string
	Interval time.Duration
}

type opsHubReport struct {
	Service   string              `json:"service"`
	Instance  string              `json:"instance"`
	Env       string              `json:"env,omitempty"`
	Timestamp int64               `json:"timestamp"`
	Chains    []opsHubChainReport `json:"chains"`
}

type opsHubChainReport struct {
	Chain        string `json:"chain"`
	Role         string `json:"role,omitempty"`
	CurrentBlock int64  `json:"current_block,omitempty"`
	LatestBlock  int64  `json:"latest_block,omitempty"`
	BlockLag     int64  `json:"block_lag,omitempty"`
	Health       string `json:"health"`
	LastError    string `json:"last_error,omitempty"`
}

// StartOpsHubReporter starts a background loop that pushes the latest chain
// health snapshot to Ops Hub. Empty Endpoint disables the reporter.
func (o *Observability) StartOpsHubReporter(cfg OpsHubReporterConfig) {
	cfg.Endpoint = strings.TrimSpace(cfg.Endpoint)
	if cfg.Endpoint == "" {
		return
	}
	if cfg.Interval <= 0 {
		cfg.Interval = defaultOpsHubReportInterval
	}
	if cfg.Env == "" {
		cfg.Env = "prod"
	}
	if cfg.Instance == "" {
		cfg.Instance = defaultInstanceName("radar")
	}

	client := &http.Client{Timeout: 5 * time.Second}
	go func() {
		ticker := time.NewTicker(cfg.Interval)
		defer ticker.Stop()

		for {
			if err := o.reportOpsHub(context.Background(), client, cfg); err != nil {
				log.Warn("Ops Hub observability report failed", "err", err)
			}

			select {
			case <-ticker.C:
			case <-o.stopCh:
				return
			}
		}
	}()
}

func (o *Observability) reportOpsHub(ctx context.Context, client *http.Client, cfg OpsHubReporterConfig) error {
	body, err := json.Marshal(o.opsHubReportPayload(cfg))
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.Endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		req.Header.Set("X-API-Key", cfg.APIKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &reportHTTPError{Status: resp.Status}
	}
	return nil
}

func (o *Observability) opsHubReportPayload(cfg OpsHubReporterConfig) opsHubReport {
	now := nowUnix()
	report := opsHubReport{
		Service:   "radar",
		Instance:  cfg.Instance,
		Env:       cfg.Env,
		Timestamp: now,
	}

	o.mu.RLock()
	for _, cs := range o.chains {
		snap := decorateSnapshot(cs.snapshot(), now)
		report.Chains = append(report.Chains, opsHubChainReport{
			Chain:        snap.Chain,
			Role:         snap.Role,
			CurrentBlock: snap.CurrentBlock,
			LatestBlock:  snap.LatestBlock,
			BlockLag:     snap.BlockLag,
			Health:       snap.Health,
			LastError:    snap.LastError,
		})
	}
	o.mu.RUnlock()
	return report
}

type reportHTTPError struct {
	Status string
}

func (e *reportHTTPError) Error() string {
	return "unexpected status from Ops Hub observability endpoint: " + e.Status
}

func defaultInstanceName(fallback string) string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		return fallback
	}
	return strings.TrimSpace(host)
}
