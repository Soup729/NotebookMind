package service

import "testing"

func TestParseReflectionResultStrictJSON(t *testing.T) {
	result := parseReflectionResult("```json\n{\"accuracy_score\":5,\"completeness_score\":4,\"source_coverage\":[\"S1\"],\"missing_aspects\":[\"timeline\"],\"suggested_improvements\":[\"cite dates\"],\"confidence_level\":\"high\"}\n```", nil)

	if result.AccuracyScore != 5 {
		t.Fatalf("expected accuracy score 5, got %d", result.AccuracyScore)
	}
	if result.CompletenessScore != 4 {
		t.Fatalf("expected completeness score 4, got %d", result.CompletenessScore)
	}
	if result.ConfidenceLevel != "high" {
		t.Fatalf("expected high confidence, got %q", result.ConfidenceLevel)
	}
	if len(result.SourceCoverage) != 1 || result.SourceCoverage[0] != "S1" {
		t.Fatalf("unexpected source coverage: %#v", result.SourceCoverage)
	}
}

func TestParseReflectionResultFallbackOnInvalidJSON(t *testing.T) {
	result := parseReflectionResult("accuracy_score: 5", []string{"S1"})

	if result.AccuracyScore != 3 {
		t.Fatalf("expected conservative fallback accuracy 3, got %d", result.AccuracyScore)
	}
	if result.ConfidenceLevel != "low" {
		t.Fatalf("expected low confidence fallback, got %q", result.ConfidenceLevel)
	}
	if len(result.MissingAspects) != 1 || result.MissingAspects[0] != "reflection parser failed" {
		t.Fatalf("unexpected fallback missing aspects: %#v", result.MissingAspects)
	}
}
