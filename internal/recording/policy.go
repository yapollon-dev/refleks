package recording

import (
	"strings"

	"refleks/internal/models"
	"refleks/internal/runs"
)

type PolicyDecision struct {
	ShouldSave bool
	Reason     models.RecordingKeepReason
	PBAtSave   bool
}

func EvaluatePolicy(cfg models.RecordingSettings, current models.RunRecord, allRuns []models.RunRecord) PolicyDecision {
	current = runs.EnsureRunID(current)
	scenario := normalizedScenario(current)

	if scenarioInList(scenario, cfg.NeverSaveScenarios) {
		return PolicyDecision{ShouldSave: false, Reason: models.RecordingKeepReasonNever}
	}
	if scenarioInList(scenario, cfg.AlwaysSaveScenarios) {
		return PolicyDecision{ShouldSave: true, Reason: models.RecordingKeepReasonAlways, PBAtSave: isNewPB(current, allRuns)}
	}

	switch cfg.SavePolicy {
	case models.RecordingPolicyManualOnly:
		return PolicyDecision{ShouldSave: false, Reason: models.RecordingKeepReasonManualOnly}
	case models.RecordingPolicyNewPB:
		if isNewPB(current, allRuns) {
			return PolicyDecision{ShouldSave: true, Reason: models.RecordingKeepReasonNewPB, PBAtSave: true}
		}
		return PolicyDecision{ShouldSave: false, Reason: models.RecordingKeepReasonNewPB}
	case models.RecordingPolicyPBTies:
		return pbOrTieDecision(current, allRuns)
	case models.RecordingPolicyTopThree:
		if isLocalTopThree(current, allRuns) {
			return PolicyDecision{ShouldSave: true, Reason: models.RecordingKeepReasonTopThree, PBAtSave: isNewPB(current, allRuns)}
		}
		return PolicyDecision{ShouldSave: false, Reason: models.RecordingKeepReasonTopThree}
	case models.RecordingPolicyEveryRun, "":
		return PolicyDecision{ShouldSave: true, Reason: models.RecordingKeepReasonEveryRun, PBAtSave: isNewPB(current, allRuns)}
	default:
		return PolicyDecision{ShouldSave: true, Reason: models.RecordingKeepReasonEveryRun, PBAtSave: isNewPB(current, allRuns)}
	}
}

func pbOrTieDecision(current models.RunRecord, allRuns []models.RunRecord) PolicyDecision {
	score := statFloat(current.Stats, "Score")
	best, found := bestPreviousScore(current, allRuns)
	if !found || score > best {
		return PolicyDecision{ShouldSave: true, Reason: models.RecordingKeepReasonNewPB, PBAtSave: true}
	}
	if score == best {
		return PolicyDecision{ShouldSave: true, Reason: models.RecordingKeepReasonPBTie, PBAtSave: true}
	}
	return PolicyDecision{ShouldSave: false, Reason: models.RecordingKeepReasonPBTie}
}

func isNewPB(current models.RunRecord, allRuns []models.RunRecord) bool {
	score := statFloat(current.Stats, "Score")
	best, found := bestPreviousScore(current, allRuns)
	return !found || score > best
}

func bestPreviousScore(current models.RunRecord, allRuns []models.RunRecord) (float64, bool) {
	current = runs.EnsureRunID(current)
	currentScenarioKey := scenarioKey(current)
	var best float64
	found := false
	for _, candidate := range allRuns {
		candidate = runs.EnsureRunID(candidate)
		if candidate.RunID == current.RunID {
			continue
		}
		if scenarioKey(candidate) != currentScenarioKey {
			continue
		}
		score := statFloat(candidate.Stats, "Score")
		if !found || score > best {
			best = score
			found = true
		}
	}
	return best, found
}

func isLocalTopThree(current models.RunRecord, allRuns []models.RunRecord) bool {
	current = runs.EnsureRunID(current)
	currentScenarioKey := scenarioKey(current)
	currentScore := statFloat(current.Stats, "Score")
	higherScores := 0
	seen := map[string]struct{}{}

	for _, candidate := range allRuns {
		candidate = runs.EnsureRunID(candidate)
		if _, ok := seen[candidate.RunID]; ok {
			continue
		}
		seen[candidate.RunID] = struct{}{}
		if scenarioKey(candidate) != currentScenarioKey {
			continue
		}
		if statFloat(candidate.Stats, "Score") > currentScore {
			higherScores++
		}
	}
	return higherScores < 3
}

func scenarioKey(rec models.RunRecord) string {
	if hash := strings.TrimSpace(statString(rec.Stats, "Hash")); hash != "" {
		return "hash:" + strings.ToLower(hash)
	}
	return "scenario:" + normalizedScenario(rec)
}

func normalizedScenario(rec models.RunRecord) string {
	if scenario := strings.TrimSpace(statString(rec.Stats, "Scenario")); scenario != "" {
		return normalizeScenarioName(scenario)
	}
	return normalizeScenarioName(rec.FileName)
}

func normalizeScenarioName(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(name)), " "))
}

func scenarioInList(scenario string, list []string) bool {
	for _, item := range list {
		if normalizeScenarioName(item) == scenario {
			return true
		}
	}
	return false
}
