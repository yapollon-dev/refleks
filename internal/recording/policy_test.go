package recording

import (
	"testing"

	"refleks/internal/models"
	"refleks/internal/runs"
)

func TestEvaluatePolicyDefaultsToEveryRun(t *testing.T) {
	current := runFixture("current", "Smoothbot", "", 100)
	decision := EvaluatePolicy(models.RecordingSettings{}, current, []models.RunRecord{current})

	if !decision.ShouldSave || decision.Reason != models.RecordingKeepReasonEveryRun {
		t.Fatalf("decision = %#v, want every run save", decision)
	}
}

func TestEvaluatePolicyNeverOverridesAlways(t *testing.T) {
	current := runFixture("current", "Smoothbot", "", 100)
	cfg := models.RecordingSettings{
		SavePolicy:          models.RecordingPolicyEveryRun,
		AlwaysSaveScenarios: []string{"Smoothbot"},
		NeverSaveScenarios:  []string{" smoothbot "},
	}

	decision := EvaluatePolicy(cfg, current, []models.RunRecord{current})
	if decision.ShouldSave || decision.Reason != models.RecordingKeepReasonNever {
		t.Fatalf("decision = %#v, want never-save scenario", decision)
	}
}

func TestEvaluatePolicyAlwaysOverridesGlobalPolicy(t *testing.T) {
	current := runFixture("current", "Smoothbot", "", 100)
	cfg := models.RecordingSettings{
		SavePolicy:          models.RecordingPolicyManualOnly,
		AlwaysSaveScenarios: []string{"Smoothbot"},
	}

	decision := EvaluatePolicy(cfg, current, []models.RunRecord{current})
	if !decision.ShouldSave || decision.Reason != models.RecordingKeepReasonAlways {
		t.Fatalf("decision = %#v, want always-save scenario", decision)
	}
}

func TestEvaluatePolicyNewPB(t *testing.T) {
	prev := runFixture("prev", "Smoothbot", "", 99)
	current := runFixture("current", "Smoothbot", "", 100)
	cfg := models.RecordingSettings{SavePolicy: models.RecordingPolicyNewPB}

	decision := EvaluatePolicy(cfg, current, []models.RunRecord{prev, current})
	if !decision.ShouldSave || decision.Reason != models.RecordingKeepReasonNewPB || !decision.PBAtSave {
		t.Fatalf("decision = %#v, want new PB", decision)
	}
}

func TestEvaluatePolicyPBTie(t *testing.T) {
	prev := runFixture("prev", "Smoothbot", "", 100)
	current := runFixture("current", "Smoothbot", "", 100)
	cfg := models.RecordingSettings{SavePolicy: models.RecordingPolicyPBTies}

	decision := EvaluatePolicy(cfg, current, []models.RunRecord{prev, current})
	if !decision.ShouldSave || decision.Reason != models.RecordingKeepReasonPBTie || !decision.PBAtSave {
		t.Fatalf("decision = %#v, want PB tie", decision)
	}
}

func TestEvaluatePolicyTopThree(t *testing.T) {
	runsForScenario := []models.RunRecord{
		runFixture("best", "Smoothbot", "", 130),
		runFixture("second", "Smoothbot", "", 120),
		runFixture("third", "Smoothbot", "", 110),
		runFixture("current", "Smoothbot", "", 109),
	}
	cfg := models.RecordingSettings{SavePolicy: models.RecordingPolicyTopThree}

	decision := EvaluatePolicy(cfg, runsForScenario[3], runsForScenario)
	if decision.ShouldSave {
		t.Fatalf("decision = %#v, want current run outside top three", decision)
	}

	current := runFixture("current", "Smoothbot", "", 110)
	decision = EvaluatePolicy(cfg, current, append(runsForScenario[:3], current))
	if !decision.ShouldSave || decision.Reason != models.RecordingKeepReasonTopThree {
		t.Fatalf("decision = %#v, want tied third to save", decision)
	}
}

func TestEvaluatePolicyManualOnlySkips(t *testing.T) {
	current := runFixture("current", "Smoothbot", "", 100)
	cfg := models.RecordingSettings{SavePolicy: models.RecordingPolicyManualOnly}

	decision := EvaluatePolicy(cfg, current, []models.RunRecord{current})
	if decision.ShouldSave || decision.Reason != models.RecordingKeepReasonManualOnly {
		t.Fatalf("decision = %#v, want manual-only skip", decision)
	}
}

func runFixture(fileName, scenario, hash string, score float64) models.RunRecord {
	rec := models.RunRecord{
		FileName: fileName,
		Stats: map[string]any{
			"Scenario": scenario,
			"Hash":     hash,
			"Score":    score,
		},
	}
	return runs.EnsureRunID(rec)
}
