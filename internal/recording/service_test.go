package recording

import (
	"context"
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

func TestTestConnectionStartsReplayBufferWhenAutoStartEnabled(t *testing.T) {
	fake := newFakeOBSServer(t, func(f *fakeOBSServer) {
		f.replayActive = false
	})
	cfg := fake.settings(t, "")
	cfg.Enabled = true
	cfg.AutoConnect = true
	cfg.AutoStartReplayBuffer = true
	service := newStorageTestService(t, cfg)

	status := service.TestConnection(context.Background())
	if status.ConnectionStatus != "connected" || status.ReplayBufferStatus != "active" {
		t.Fatalf("test connection should start replay buffer, got %#v", status)
	}
	if status.LastError != "" {
		t.Fatalf("unexpected status error: %#v", status)
	}
}

func TestSaveRunReplayRecordsCaptureTimingMetadata(t *testing.T) {
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

	record, err := service.saveRunReplay(context.Background(), run, models.RecordingKeepReasonEveryRun, false, importedAt)
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

	record, err := service.saveRunReplay(context.Background(), recordingRunFixture(), models.RecordingKeepReasonEveryRun, false, time.Now().UTC())
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

	record, err := service.saveRunReplay(context.Background(), run, models.RecordingKeepReasonEveryRun, false, time.Now().UTC())
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
	return runs.EnsureRunID(models.RunRecord{
		FilePath: "C:/runs/run.refleks",
		FileName: "Smoothbot - Challenge - 2026.06.19-10.00.00",
		Stats: map[string]any{
			"Scenario":    "Smoothbot",
			"Score":       123.4,
			"Date Played": "2026-06-19T10:00:00Z",
			"Duration":    60.0,
		},
	})
}
