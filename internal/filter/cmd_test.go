package filter

import (
	"testing"

	"github.com/mapprotocol/filter/internal/filter/config"
)

func TestOpsHubObservabilityInstanceDefaultsToEnv(t *testing.T) {
	got := opsHubObservabilityInstance(config.Construction{Env: "dev"})
	if got != "dev" {
		t.Fatalf("instance = %q, want dev", got)
	}
}

func TestOpsHubObservabilityInstanceAllowsOverride(t *testing.T) {
	got := opsHubObservabilityInstance(config.Construction{
		Env:                         "dev",
		OpsHubObservabilityInstance: "radar-bsc-01",
	})
	if got != "radar-bsc-01" {
		t.Fatalf("instance = %q, want radar-bsc-01", got)
	}
}
