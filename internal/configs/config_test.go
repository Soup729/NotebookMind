package configs

import "testing"

func TestCitationGuardDefaults(t *testing.T) {
	t.Setenv("ENABLE_CITATION_GUARD", "")
	t.Setenv("CITATION_GUARD_HIGH_RISK_ONLY", "")
	t.Setenv("CITATION_GUARD_REPAIR_ENABLED", "")
	t.Setenv("CITATION_GUARD_MAX_REPAIR_ATTEMPTS", "")
	t.Setenv("CITATION_GUARD_MIN_COVERAGE", "")

	cfg := Config{
		Parser: ParserConfig{
			ChunkSize:      1000,
			ChunkOverlap:   200,
			ChildChunkSize: 300,
		},
	}
	overrideFromEnv(&cfg)

	if !cfg.CitationGuard.Enabled {
		t.Fatalf("expected citation guard to default enabled")
	}
	if cfg.CitationGuard.RepairEnabled {
		t.Fatalf("expected citation guard repair to default disabled for latency")
	}
	if cfg.CitationGuard.MaxRepairAttempts != 0 {
		t.Fatalf("expected zero repair attempts by default, got %d", cfg.CitationGuard.MaxRepairAttempts)
	}
	if cfg.CitationGuard.MinCitationCoverage != 0.8 {
		t.Fatalf("expected min citation coverage 0.8, got %f", cfg.CitationGuard.MinCitationCoverage)
	}
}
