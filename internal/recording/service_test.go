package recording

import (
	"context"
	"path/filepath"
	"testing"

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
