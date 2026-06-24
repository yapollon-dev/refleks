package recording

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"refleks/internal/models"
)

type runClipWindow struct {
	ScenarioStart  time.Time
	ScenarioEnd    time.Time
	RequestedStart time.Time
	RequestedEnd   time.Time
	Source         string
	PreRoll        time.Duration
	PostRoll       time.Duration
}

type replayClipPlan struct {
	ReplayStart    time.Time
	ReplayEnd      time.Time
	ActualStart    time.Time
	ActualEnd      time.Time
	Offset         time.Duration
	Duration       time.Duration
	TruncatedStart bool
	TruncatedEnd   bool
}

// determineRunClipWindow finds verified scenario boundaries before adding user-configured padding.
func determineRunClipWindow(rec models.RunRecord, cfg models.RecordingSettings) (runClipWindow, error) {
	stats := rec.Stats
	if stats == nil {
		return runClipWindow{}, errors.New("run has no stats available for clip timing")
	}

	preRoll := time.Duration(cfg.PreRollSeconds) * time.Second
	postRoll := time.Duration(cfg.PostRollSeconds) * time.Second
	if preRoll < 0 {
		preRoll = 0
	}
	if postRoll < 0 {
		postRoll = 0
	}

	if start, startKey, ok := firstAbsoluteStat(stats, "Scenario Start", "Scenario Start Time", "Start Time", "Start Timestamp", "Challenge Start Timestamp"); ok {
		if end, endKey, ok := firstAbsoluteStat(stats, "Scenario End", "Scenario End Time", "End Time", "End Timestamp", "Date Played"); ok {
			return buildRunClipWindow(start, end, preRoll, postRoll, "direct_absolute_stats:"+startKey+"+"+endKey)
		}
	}

	// Kovaak's Stats.csv gives Challenge Start as a time-of-day and the filename gives Date Played as completion time.
	end, endKey, ok := firstAbsoluteStat(stats, "Date Played")
	if !ok {
		return runClipWindow{}, errors.New("run has no absolute Date Played timestamp for clip timing")
	}

	challengeStart := statString(stats, "Challenge Start")
	if challengeStart == "" {
		return runClipWindow{}, errors.New("run has no Challenge Start timestamp; exact clip boundaries cannot be established")
	}
	start, ok := parseTimeOfDayOnDate(challengeStart, end)
	if !ok {
		return runClipWindow{}, fmt.Errorf("run Challenge Start %q is not a recognized time format", challengeStart)
	}
	if start.After(end) {
		start = start.AddDate(0, 0, -1)
	}
	return buildRunClipWindow(start, end, preRoll, postRoll, "challenge_start_time_of_day+"+endKey+"_completion")
}

func buildRunClipWindow(start, end time.Time, preRoll, postRoll time.Duration, source string) (runClipWindow, error) {
	if start.IsZero() || end.IsZero() {
		return runClipWindow{}, errors.New("run clip boundaries are incomplete")
	}
	if !end.After(start) {
		return runClipWindow{}, fmt.Errorf("run clip end %s is not after start %s", end.Format(time.RFC3339Nano), start.Format(time.RFC3339Nano))
	}
	return runClipWindow{
		ScenarioStart:  start,
		ScenarioEnd:    end,
		RequestedStart: start.Add(-preRoll),
		RequestedEnd:   end.Add(postRoll),
		Source:         source,
		PreRoll:        preRoll,
		PostRoll:       postRoll,
	}, nil
}

// buildReplayClipPlan maps absolute run timestamps into the saved replay file timeline and clamps missing edges.
func buildReplayClipPlan(window runClipWindow, replayEnd time.Time, replayDuration time.Duration) (replayClipPlan, error) {
	if replayEnd.IsZero() {
		return replayClipPlan{}, errors.New("OBS replay timeline end is unknown")
	}
	if replayDuration <= 0 {
		return replayClipPlan{}, errors.New("OBS replay duration is unknown")
	}

	replayStart := replayEnd.Add(-replayDuration)
	actualStart := maxTime(window.RequestedStart, replayStart)
	actualEnd := minTime(window.RequestedEnd, replayEnd)
	if !actualEnd.After(actualStart) {
		return replayClipPlan{}, errors.New("OBS replay does not overlap the requested run clip window")
	}

	return replayClipPlan{
		ReplayStart:    replayStart,
		ReplayEnd:      replayEnd,
		ActualStart:    actualStart,
		ActualEnd:      actualEnd,
		Offset:         actualStart.Sub(replayStart),
		Duration:       actualEnd.Sub(actualStart),
		TruncatedStart: actualStart.After(window.RequestedStart),
		TruncatedEnd:   actualEnd.Before(window.RequestedEnd),
	}, nil
}

func firstAbsoluteStat(stats map[string]any, keys ...string) (time.Time, string, bool) {
	for _, key := range keys {
		raw := statString(stats, key)
		if raw == "" {
			continue
		}
		if parsed, ok := parseAbsoluteTime(raw); ok {
			return parsed, key, true
		}
	}
	return time.Time{}, "", false
}

func parseAbsoluteTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05.999999999 -0700",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006.01.02-15.04.05",
	}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func parseTimeOfDayOnDate(raw string, date time.Time) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	layouts := []string{
		"15:04:05.000000",
		"15:04:05.000",
		"15:04:05",
	}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return time.Date(date.Year(), date.Month(), date.Day(), parsed.Hour(), parsed.Minute(), parsed.Second(), parsed.Nanosecond(), date.Location()), true
		}
	}
	return time.Time{}, false
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
