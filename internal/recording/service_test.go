package recording

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"refleks/internal/models"
	"refleks/internal/runs"
)

func TestAutoMetadataExistsForRunTreatsSkippedAsHandled(t *testing.T) {
	store := NewMetadataStore(filepath.Join(t.TempDir(), "recordings.json"))
	service := &Service{metadata: store}
	if err := store.Save([]models.RecordingRecord{
		{ID: "skipped", RunID: "run-1", Status: models.RecordingStatusSkipped},
	}); err != nil {
		t.Fatalf("save fixtures: %v", err)
	}

	exists, err := service.autoMetadataExistsForRun("run-1")
	if err != nil {
		t.Fatalf("auto metadata check failed: %v", err)
	}
	if !exists {
		t.Fatalf("skipped auto metadata should prevent repeated auto handling")
	}
}

func TestAutoMetadataExistsForRunIgnoresFailedRecords(t *testing.T) {
	store := NewMetadataStore(filepath.Join(t.TempDir(), "recordings.json"))
	service := &Service{metadata: store}
	if err := store.Save([]models.RecordingRecord{
		{ID: "failed", RunID: "run-1", Status: models.RecordingStatusFailed},
	}); err != nil {
		t.Fatalf("save fixtures: %v", err)
	}

	exists, err := service.autoMetadataExistsForRun("run-1")
	if err != nil {
		t.Fatalf("auto metadata check failed: %v", err)
	}
	if exists {
		t.Fatalf("failed records should not block later auto handling")
	}
}

func TestNewRecordingRecordLinksRunAndReason(t *testing.T) {
	run := runs.EnsureRunID(models.RunRecord{
		FilePath: "C:/runs/run.refleks",
		FileName: "Smoothbot - Challenge - 2026.06.19-10.00.00",
		Stats: map[string]any{
			"Scenario":    "Smoothbot",
			"Score":       123.4,
			"Date Played": "2026-06-19T10:00:00Z",
		},
	})

	record := newRecordingRecord(run, models.RecordingKeepReasonTopThree, true)
	if record.RunID != run.RunID || record.RunFilePath != run.FilePath || record.RunFileName != run.FileName {
		t.Fatalf("record does not preserve run link: %#v", record)
	}
	if record.KeepReason != models.RecordingKeepReasonTopThree || !record.PBAtSave {
		t.Fatalf("record reason/PB flag mismatch: %#v", record)
	}
	if record.Status != models.RecordingStatusPending {
		t.Fatalf("status = %q, want pending", record.Status)
	}
}

func TestTestConnectionDoesNotStartReplayBufferWhenAutoStartEnabled(t *testing.T) {
	fake := newFakeOBSServer(t, func(f *fakeOBSServer) {
		f.replayActive = false
	})
	cfg := fake.settings(t, "")
	cfg.Enabled = true
	cfg.AutoConnect = true
	cfg.AutoStartReplayBuffer = true
	service := newStorageTestService(t, cfg)

	status := service.TestConnection(context.Background())
	if status.ConnectionStatus != "not_connected" || status.LastConnectionStatus != "connected" || status.ReplayBufferStatus != "inactive" {
		t.Fatalf("test connection should report read-only status, got %#v", status)
	}
	if status.LastError != "" {
		t.Fatalf("unexpected status error: %#v", status)
	}
	if fake.startReplayRequests != 0 {
		t.Fatalf("test connection should not start replay buffer, got %d start requests", fake.startReplayRequests)
	}
}

func TestStartReplayBufferStartsInactiveBuffer(t *testing.T) {
	fake := newFakeOBSServer(t, func(f *fakeOBSServer) {
		f.replayActive = false
	})
	cfg := fake.settings(t, "")
	service := newStorageTestService(t, cfg)

	status := service.StartReplayBuffer(context.Background())
	if status.ConnectionStatus != "not_connected" || status.LastConnectionStatus != "connected" || status.ReplayBufferStatus != "active" {
		t.Fatalf("start replay buffer status mismatch: %#v", status)
	}
	if status.LastError != "" {
		t.Fatalf("unexpected status error: %#v", status)
	}
	if fake.startReplayRequests != 1 {
		t.Fatalf("explicit start should call StartReplayBuffer once, got %d", fake.startReplayRequests)
	}
}

func TestServiceChangeCallbackFiresOnMetadataWrites(t *testing.T) {
	service := newStorageTestService(t, models.RecordingSettings{RecordingDir: t.TempDir()})
	changes := 0
	service.SetOnChanged(func() {
		changes++
	})

	record := models.RecordingRecord{ID: "recording-1", Status: models.RecordingStatusSaved, VideoPath: "clip.mp4", SizeBytes: 1}
	if err := service.upsertRecord(record); err != nil {
		t.Fatalf("upsert record: %v", err)
	}
	if err := service.saveRecords([]models.RecordingRecord{record}); err != nil {
		t.Fatalf("save records: %v", err)
	}
	if changes != 2 {
		t.Fatalf("change callback count = %d, want 2", changes)
	}
}

func TestSaveRunReplayRecordsCaptureTimingMetadata(t *testing.T) {
	trimPlan := withFakeVideoTrim(t, 90*time.Second, []byte("trimmed clip"))
	replayPath := filepath.Join(t.TempDir(), "confirmed replay.mp4")
	fake := newFakeOBSServer(t, func(f *fakeOBSServer) {
		f.replayActive = true
		f.replayPath = replayPath
		f.writeReplayOnSave = true
	})
	cfg := fake.settings(t, "")
	cfg.Enabled = true
	cfg.AutoStartReplayBuffer = true
	cfg.RecordingDir = t.TempDir()
	service := newStorageTestService(t, cfg)
	run := recordingRunFixture()
	importedAt := time.Now().UTC().Add(-100 * time.Millisecond)

	record, err := service.saveRunReplay(context.Background(), run, models.RecordingKeepReasonEveryRun, false, importedAt, models.RecordingLinkSourceAutoCompletedRun, true)
	if err != nil {
		t.Fatalf("save replay: %v", err)
	}
	if record.Status != models.RecordingStatusSaved {
		t.Fatalf("status = %q, want saved", record.Status)
	}
	if record.RunImportedAt == "" || record.CaptureRequestedAt == "" || record.CaptureConfirmedAt == "" || record.OBSReplayFileModTime == "" {
		t.Fatalf("record should include capture timing metadata: %#v", record)
	}
	if record.ReplayBufferStatusAtSave != "active" {
		t.Fatalf("replay buffer status = %q, want active", record.ReplayBufferStatusAtSave)
	}
	if record.CaptureDelayMs <= 0 {
		t.Fatalf("capture delay should be recorded, got %d", record.CaptureDelayMs)
	}
	if record.OBSSourcePath != replayPath || record.VideoPath == "" {
		t.Fatalf("record should keep source and final paths: %#v", record)
	}
	if record.ActiveVideoKind != models.RecordingVideoKindTrimmed || record.TrimStatus != models.RecordingTrimStatusSucceeded {
		t.Fatalf("record should use validated trimmed clip: %#v", record)
	}
	if record.TimingSource == "" || record.ScenarioStartAt == "" || record.ReplayTimelineStartAt == "" {
		t.Fatalf("record should include run/replay timing metadata: %#v", record)
	}
	if trimPlan == nil || trimPlan.Duration <= 0 {
		t.Fatalf("trim plan should be captured")
	}
	if _, err := os.Stat(record.VideoPath); err != nil {
		t.Fatalf("trimmed video should exist: %v", err)
	}
	if _, err := os.Stat(record.RawVideoPath); !os.IsNotExist(err) {
		t.Fatalf("raw replay copy should be deleted by default after successful trim, stat error = %v", err)
	}
	if _, err := os.Stat(replayPath); !os.IsNotExist(err) {
		t.Fatalf("OBS source replay should be deleted after successful trim, stat error = %v", err)
	}
}

func TestSaveRunReplayFailsWhenReplayBufferWasInactiveAtSaveTime(t *testing.T) {
	replayPath := filepath.Join(t.TempDir(), "unsafe replay.mp4")
	fake := newFakeOBSServer(t, func(f *fakeOBSServer) {
		f.replayActive = false
		f.replayPath = replayPath
		f.writeReplayOnSave = true
	})
	cfg := fake.settings(t, "")
	cfg.Enabled = true
	cfg.AutoStartReplayBuffer = true
	cfg.RecordingDir = t.TempDir()
	service := newStorageTestService(t, cfg)

	record, err := service.saveRunReplay(context.Background(), recordingRunFixture(), models.RecordingKeepReasonEveryRun, false, time.Now().UTC(), models.RecordingLinkSourceAutoCompletedRun, true)
	if err == nil {
		t.Fatalf("save should fail when replay buffer was inactive")
	}
	if !strings.Contains(err.Error(), "not active before this save") {
		t.Fatalf("error should explain unsafe inactive buffer, got %v", err)
	}
	if record.Status != models.RecordingStatusFailed {
		t.Fatalf("status = %q, want failed", record.Status)
	}
	if record.ReplayBufferStatusAtSave != "started_after_save_request" {
		t.Fatalf("replay buffer status = %q, want started_after_save_request", record.ReplayBufferStatusAtSave)
	}
	if fake.startReplayRequests != 1 || fake.saveReplayRequests != 0 {
		t.Fatalf("expected one start and no save, got start=%d save=%d", fake.startReplayRequests, fake.saveReplayRequests)
	}
}

func TestSaveRunReplayRetriesFailedRecordInPlace(t *testing.T) {
	withFakeVideoTrim(t, 90*time.Second, []byte("retry trim"))
	replayPath := filepath.Join(t.TempDir(), "retry replay.mp4")
	fake := newFakeOBSServer(t, func(f *fakeOBSServer) {
		f.replayActive = true
		f.replayPath = replayPath
		f.writeReplayOnSave = true
	})
	cfg := fake.settings(t, "")
	cfg.Enabled = true
	cfg.AutoStartReplayBuffer = true
	cfg.RecordingDir = t.TempDir()
	service := newStorageTestService(t, cfg)
	run := recordingRunFixture()
	existing := newRecordingRecord(run, models.RecordingKeepReasonEveryRun, false)
	existing.ID = "rec_existing"
	existing.Status = models.RecordingStatusFailed
	existing.LastError = "previous failure"
	existing.Protected = true
	if err := service.metadata.Save([]models.RecordingRecord{existing}); err != nil {
		t.Fatalf("save existing failed recording: %v", err)
	}

	record, err := service.saveRunReplay(context.Background(), run, models.RecordingKeepReasonEveryRun, false, time.Now().UTC(), models.RecordingLinkSourceAutoCompletedRun, true)
	if err != nil {
		t.Fatalf("retry save replay: %v", err)
	}
	if record.ID != existing.ID {
		t.Fatalf("retry should reuse recording id %q, got %q", existing.ID, record.ID)
	}
	if !record.Protected {
		t.Fatalf("retry should preserve protection flag")
	}
	if record.Status != models.RecordingStatusSaved || record.LastError != "" {
		t.Fatalf("retry should save and clear previous error: %#v", record)
	}
	records, err := service.metadata.List()
	if err != nil {
		t.Fatalf("list metadata: %v", err)
	}
	if len(records) != 1 || records[0].ID != existing.ID {
		t.Fatalf("retry should update one metadata row, got %#v", records)
	}
}

func TestSaveRunReplayCanKeepRawReplayAfterSuccessfulTrim(t *testing.T) {
	withFakeVideoTrim(t, 90*time.Second, []byte("trimmed with raw retained"))
	replayPath := filepath.Join(t.TempDir(), "keep raw replay.mp4")
	fake := newFakeOBSServer(t, func(f *fakeOBSServer) {
		f.replayActive = true
		f.replayPath = replayPath
		f.writeReplayOnSave = true
	})
	cfg := fake.settings(t, "")
	cfg.Enabled = true
	cfg.AutoStartReplayBuffer = true
	cfg.KeepRawReplay = true
	cfg.RecordingDir = t.TempDir()
	service := newStorageTestService(t, cfg)

	record, err := service.saveRunReplay(context.Background(), recordingRunFixture(), models.RecordingKeepReasonEveryRun, false, time.Now().UTC(), models.RecordingLinkSourceAutoCompletedRun, true)
	if err != nil {
		t.Fatalf("save replay: %v", err)
	}
	if record.ActiveVideoKind != models.RecordingVideoKindTrimmed || record.TrimStatus != models.RecordingTrimStatusSucceeded {
		t.Fatalf("record should use trimmed clip: %#v", record)
	}
	if _, err := os.Stat(record.RawVideoPath); err != nil {
		t.Fatalf("raw replay copy should remain when retention is enabled: %v", err)
	}
	if _, err := os.Stat(record.TrimmedVideoPath); err != nil {
		t.Fatalf("trimmed replay should exist: %v", err)
	}
	if _, err := os.Stat(replayPath); !os.IsNotExist(err) {
		t.Fatalf("OBS source replay should still be deleted after successful trim, stat error = %v", err)
	}
}

func TestSaveCurrentReplayDefaultsToUnlinkedAndIgnoresAutoEnabled(t *testing.T) {
	replayPath := filepath.Join(t.TempDir(), "manual unlinked.mp4")
	fake := newFakeOBSServer(t, func(f *fakeOBSServer) {
		f.replayActive = true
		f.replayPath = replayPath
		f.writeReplayOnSave = true
	})
	cfg := fake.settings(t, "")
	cfg.Enabled = false
	cfg.AutoStartReplayBuffer = true
	cfg.RecordingDir = t.TempDir()
	service := newStorageTestService(t, cfg)

	record, err := service.SaveCurrentReplay(context.Background(), "")
	if err != nil {
		t.Fatalf("save unlinked replay: %v", err)
	}
	if record.Status != models.RecordingStatusSaved {
		t.Fatalf("status = %q, want saved", record.Status)
	}
	if record.RunID != "" || record.RunFileName != "" || record.LinkSource != models.RecordingLinkSourceUnlinked {
		t.Fatalf("manual save should be unlinked by default: %#v", record)
	}
	if record.KeepReason != models.RecordingKeepReasonManual {
		t.Fatalf("keep reason = %q, want manual_save", record.KeepReason)
	}
	if record.VideoPath == "" {
		t.Fatalf("manual replay should have final video path")
	}
}

func TestSaveCurrentReplayLinksOnlyExplicitSelectedRun(t *testing.T) {
	withFakeVideoTrim(t, 90*time.Second, []byte("manual selected trim"))
	replayPath := filepath.Join(t.TempDir(), "manual linked.mp4")
	fake := newFakeOBSServer(t, func(f *fakeOBSServer) {
		f.replayActive = true
		f.replayPath = replayPath
		f.writeReplayOnSave = true
	})
	cfg := fake.settings(t, "")
	cfg.Enabled = false
	cfg.AutoStartReplayBuffer = true
	cfg.RecordingDir = t.TempDir()
	service := newStorageTestService(t, cfg)
	run := recordingRunFixture()
	runPath, err := service.runStore.Save(runs.RunRecord{
		FileName: run.FileName,
		Stats:    run.Stats,
	})
	if err != nil {
		t.Fatalf("save run fixture: %v", err)
	}

	record, err := service.SaveCurrentReplay(context.Background(), run.RunID)
	if err != nil {
		t.Fatalf("save linked replay: %v", err)
	}
	if record.RunID != run.RunID || record.RunFilePath != runPath {
		t.Fatalf("manual selected save should link selected run: %#v", record)
	}
	if record.LinkSource != models.RecordingLinkSourceManualSelected {
		t.Fatalf("link source = %q, want manual_user_selected", record.LinkSource)
	}
}

func TestSaveLatestRunReplayCompatibilitySavesUnlinked(t *testing.T) {
	replayPath := filepath.Join(t.TempDir(), "compat unlinked.mp4")
	fake := newFakeOBSServer(t, func(f *fakeOBSServer) {
		f.replayActive = true
		f.replayPath = replayPath
		f.writeReplayOnSave = true
	})
	cfg := fake.settings(t, "")
	cfg.Enabled = false
	cfg.AutoStartReplayBuffer = true
	cfg.RecordingDir = t.TempDir()
	service := newStorageTestService(t, cfg)

	record, err := service.SaveLatestRunReplay(context.Background())
	if err != nil {
		t.Fatalf("compat save replay: %v", err)
	}
	if record.RunID != "" || record.LinkSource != models.RecordingLinkSourceUnlinked {
		t.Fatalf("compat latest API should not infer a run link: %#v", record)
	}
}

func TestHandleCompletedRunAutoConnectDisabledUpdatesFailedRecordInPlace(t *testing.T) {
	cfg := models.RecordingSettings{
		Enabled:      true,
		AutoConnect:  false,
		RecordingDir: t.TempDir(),
	}
	service := newStorageTestService(t, cfg)
	run := recordingRunFixture()

	first, err := service.HandleCompletedRun(context.Background(), run)
	if err == nil {
		t.Fatalf("first auto attempt should fail when auto connect is disabled")
	}
	if first.Status != models.RecordingStatusFailed {
		t.Fatalf("first status = %q, want failed", first.Status)
	}

	second, err := service.HandleCompletedRun(context.Background(), run)
	if err == nil {
		t.Fatalf("second auto attempt should still report auto connect disabled")
	}
	if second.ID != first.ID {
		t.Fatalf("duplicate auto failure should reuse id %q, got %q", first.ID, second.ID)
	}
	records, err := service.metadata.List()
	if err != nil {
		t.Fatalf("list metadata: %v", err)
	}
	if len(records) != 1 || records[0].ID != first.ID {
		t.Fatalf("duplicate auto failure should update one row, got %#v", records)
	}
}

func TestHandleCompletedRunPolicySkipDoesNotPersistMetadata(t *testing.T) {
	cfg := models.RecordingSettings{
		Enabled:      true,
		AutoConnect:  true,
		SavePolicy:   models.RecordingPolicyNewPB,
		RecordingDir: t.TempDir(),
	}
	service := newStorageTestService(t, cfg)
	run := recordingRunFixture()
	higher := run
	higher.FileName = "Smoothbot - Challenge - 2026.06.19-09.00.00"
	higher.Stats = map[string]any{
		"Scenario":    "Smoothbot",
		"Score":       999.0,
		"Date Played": "2026-06-19T09:00:00Z",
	}
	if _, err := service.runStore.Save(runs.RunRecord{FileName: higher.FileName, Stats: higher.Stats}); err != nil {
		t.Fatalf("save higher run fixture: %v", err)
	}

	record, err := service.HandleCompletedRun(context.Background(), run)
	if err != nil {
		t.Fatalf("policy skip should not fail: %v", err)
	}
	if record.ID != "" {
		t.Fatalf("policy skip should not return persisted metadata: %#v", record)
	}
	records, err := service.metadata.List()
	if err != nil {
		t.Fatalf("list metadata: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("policy skip should not persist metadata rows, got %#v", records)
	}
}

func TestEnsureReplayBufferStartedNoopsWhenRecordingDisabled(t *testing.T) {
	fake := newFakeOBSServer(t, func(f *fakeOBSServer) {
		f.replayActive = false
	})
	cfg := fake.settings(t, "")
	cfg.Enabled = false
	cfg.AutoConnect = true
	cfg.AutoStartReplayBuffer = true
	service := newStorageTestService(t, cfg)

	status := service.EnsureReplayBufferStarted(context.Background())
	if status.ConnectionStatus == "connected" || status.ReplayBufferStatus == "active" {
		t.Fatalf("disabled recording should not connect/start replay buffer: %#v", status)
	}
}

func recordingRunFixture() models.RunRecord {
	playedAt := time.Now().UTC().Add(-5 * time.Second)
	startedAt := playedAt.Add(-60 * time.Second)
	return runs.EnsureRunID(models.RunRecord{
		FilePath: "C:/runs/run.refleks",
		FileName: "Smoothbot - Challenge - 2026.06.19-10.00.00",
		Stats: map[string]any{
			"Scenario":        "Smoothbot",
			"Score":           123.4,
			"Date Played":     playedAt.Format(time.RFC3339Nano),
			"Challenge Start": startedAt.Format("15:04:05.000"),
			"Duration":        60.0,
		},
	})
}

func withFakeVideoTrim(t *testing.T, rawDuration time.Duration, payload []byte) *replayClipPlan {
	t.Helper()
	previousFind := findVideoToolchain
	previousProbe := probeVideoDuration
	previousTrim := trimReplayVideo
	var captured replayClipPlan
	findVideoToolchain = func() (videoToolchain, error) {
		return videoToolchain{FFmpeg: "fake-ffmpeg", FFprobe: "fake-ffprobe"}, nil
	}
	probeVideoDuration = func(ctx context.Context, toolchain videoToolchain, path string) (time.Duration, error) {
		return rawDuration, nil
	}
	trimReplayVideo = func(ctx context.Context, toolchain videoToolchain, sourcePath, targetPath string, plan replayClipPlan) (int64, time.Duration, error) {
		if _, err := os.Stat(sourcePath); err != nil {
			return 0, 0, err
		}
		if err := os.WriteFile(targetPath, payload, 0o644); err != nil {
			return 0, 0, err
		}
		captured = plan
		return int64(len(payload)), plan.Duration, nil
	}
	t.Cleanup(func() {
		findVideoToolchain = previousFind
		probeVideoDuration = previousProbe
		trimReplayVideo = previousTrim
	})
	return &captured
}
