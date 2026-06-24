package runs

import "testing"

func TestComputeRunIDIsStable(t *testing.T) {
	stats := map[string]any{
		"Scenario":    "1w6ts reload v2",
		"Hash":        "abc123",
		"Date Played": "2026-05-02T10:14:14+02:00",
		"Score":       1234.5,
	}

	first := ComputeRunID("1w6ts reload v2 - Challenge - 2026.05.02-10.14.14", stats)
	second := ComputeRunID("1w6ts reload v2 - Challenge - 2026.05.02-10.14.14", stats)

	if first == "" {
		t.Fatalf("run ID should not be empty")
	}
	if first != second {
		t.Fatalf("run ID should be stable: %q != %q", first, second)
	}
}

func TestComputeRunIDChangesForDifferentRuns(t *testing.T) {
	stats := map[string]any{
		"Scenario":    "1w6ts reload v2",
		"Hash":        "abc123",
		"Date Played": "2026-05-02T10:14:14+02:00",
		"Score":       1234.5,
	}

	first := ComputeRunID("1w6ts reload v2 - Challenge - 2026.05.02-10.14.14", stats)
	stats["Score"] = 1235.5
	second := ComputeRunID("1w6ts reload v2 - Challenge - 2026.05.02-10.14.14", stats)

	if first == second {
		t.Fatalf("run ID should change when identifying run fields change")
	}
}
