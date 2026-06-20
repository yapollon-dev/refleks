package recording

import (
	"context"
	"strings"
	"testing"
	"time"

	"refleks/internal/models"
)

func TestDetermineRunClipWindowUsesChallengeStartAndDatePlayed(t *testing.T) {
	rec := models.RunRecord{
		Stats: map[string]any{
			"Date Played":     "2026-05-02T10:14:14+02:00",
			"Challenge Start": "10:13:14.228",
		},
	}
	cfg := models.RecordingSettings{PreRollSeconds: 2, PostRollSeconds: 3}

	window, err := determineRunClipWindow(rec, cfg)
	if err != nil {
		t.Fatalf("determine window: %v", err)
	}
	if window.Source != "challenge_start_time_of_day+Date Played_completion" {
		t.Fatalf("source = %q", window.Source)
	}
	if got := window.ScenarioStart.Format("15:04:05.000"); got != "10:13:14.228" {
		t.Fatalf("start = %s, want 10:13:14.228", got)
	}
	if got := window.RequestedStart.Format("15:04:05.000"); got != "10:13:12.228" {
		t.Fatalf("requested start = %s, want pre-roll applied", got)
	}
	if got := window.RequestedEnd.Format("15:04:05.000"); got != "10:14:17.000" {
		t.Fatalf("requested end = %s, want post-roll applied", got)
	}
}

func TestDetermineRunClipWindowRejectsMissingChallengeStart(t *testing.T) {
	rec := models.RunRecord{Stats: map[string]any{"Date Played": "2026-05-02T10:14:14+02:00"}}

	_, err := determineRunClipWindow(rec, models.RecordingSettings{PreRollSeconds: 2, PostRollSeconds: 3})
	if err == nil || !strings.Contains(err.Error(), "Challenge Start") {
		t.Fatalf("expected missing Challenge Start error, got %v", err)
	}
}

func TestBuildReplayClipPlanMarksTruncatedStart(t *testing.T) {
	start := time.Date(2026, 5, 2, 10, 13, 14, 0, time.UTC)
	window := runClipWindow{
		ScenarioStart:  start,
		ScenarioEnd:    start.Add(60 * time.Second),
		RequestedStart: start.Add(-2 * time.Second),
		RequestedEnd:   start.Add(63 * time.Second),
	}
	replayEnd := start.Add(63 * time.Second)

	plan, err := buildReplayClipPlan(window, replayEnd, 30*time.Second)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if !plan.TruncatedStart || plan.TruncatedEnd {
		t.Fatalf("truncation flags = start:%v end:%v", plan.TruncatedStart, plan.TruncatedEnd)
	}
	if plan.Offset != 0 {
		t.Fatalf("offset = %v, want beginning of available replay", plan.Offset)
	}
}

func TestBuildReplayClipPlanMarksTruncatedEnd(t *testing.T) {
	start := time.Date(2026, 5, 2, 10, 13, 14, 0, time.UTC)
	window := runClipWindow{
		ScenarioStart:  start,
		ScenarioEnd:    start.Add(60 * time.Second),
		RequestedStart: start.Add(-2 * time.Second),
		RequestedEnd:   start.Add(63 * time.Second),
	}
	replayEnd := start.Add(61 * time.Second)

	plan, err := buildReplayClipPlan(window, replayEnd, 90*time.Second)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if plan.TruncatedStart || !plan.TruncatedEnd {
		t.Fatalf("truncation flags = start:%v end:%v", plan.TruncatedStart, plan.TruncatedEnd)
	}
	if plan.ActualEnd != replayEnd {
		t.Fatalf("actual end should be clamped to replay end")
	}
}

func TestWaitForRunClipEndRespectsContextCancellation(t *testing.T) {
	end := time.Now().Add(30 * time.Second)
	rec := models.RunRecord{
		Stats: map[string]any{
			"Date Played":     end.Format(time.RFC3339Nano),
			"Challenge Start": end.Add(-60 * time.Second).Format("15:04:05.000"),
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := waitForRunClipEnd(ctx, rec, models.RecordingSettings{PreRollSeconds: 2, PostRollSeconds: 3})
	if err == nil {
		t.Fatalf("expected canceled context error")
	}
}
