package observability

import (
	"encoding/json"
	"testing"
	"time"
)

func TestOpsHubReportPayloadUsesChainSnapshots(t *testing.T) {
	defer freezeTime(1700000000)()
	o := New("test", Config{})
	cs := o.RegisterChain("bsc", "sync")
	cs.SetLatestBlock(100)
	cs.SetCurrentBlock(90)

	payload := o.opsHubReportPayload(OpsHubReporterConfig{
		Env:      "dev",
		Instance: "radar-bsc-01",
	})
	if payload.Service != "radar" {
		t.Fatalf("service = %q, want radar", payload.Service)
	}
	if payload.Instance != "radar-bsc-01" {
		t.Fatalf("instance = %q, want radar-bsc-01", payload.Instance)
	}
	if payload.Env != "dev" {
		t.Fatalf("env = %q, want dev", payload.Env)
	}
	if payload.Timestamp != 1700000000 {
		t.Fatalf("timestamp = %d, want 1700000000", payload.Timestamp)
	}
	if len(payload.Chains) != 1 {
		t.Fatalf("chains len = %d, want 1", len(payload.Chains))
	}
	got := payload.Chains[0]
	if got.Chain != "bsc" || got.Role != "sync" || got.CurrentBlock != 90 || got.LatestBlock != 100 || got.BlockLag != 10 {
		t.Fatalf("chain payload mismatch: %+v", got)
	}
	if got.Health == "" {
		t.Fatalf("health should be populated: %+v", got)
	}
	if _, err := json.Marshal(payload); err != nil {
		t.Fatalf("payload should marshal: %v", err)
	}
}

func TestOpsHubReporterDefaults(t *testing.T) {
	if defaultOpsHubReportInterval != 15*time.Second {
		t.Fatalf("default interval = %s, want 15s", defaultOpsHubReportInterval)
	}
}
