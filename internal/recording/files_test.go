package recording

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"refleks/internal/models"
)

func TestMoveRecordingFileMovesStableFile(t *testing.T) {
	oldTimeout := fileStableTimeout
	oldInterval := fileStableInterval
	fileStableTimeout = time.Second
	fileStableInterval = 10 * time.Millisecond
	t.Cleanup(func() {
		fileStableTimeout = oldTimeout
		fileStableInterval = oldInterval
	})

	dir := t.TempDir()
	source := filepath.Join(dir, "obs replay.mp4")
	if err := os.WriteFile(source, []byte("clip"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	target, size, err := moveRecordingFile(source, filepath.Join(dir, "recordings"), "run:name")
	if err != nil {
		t.Fatalf("move failed: %v", err)
	}
	if size != 4 {
		t.Fatalf("size = %d, want 4", size)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("target missing: %v", err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("source should be moved, stat err = %v", err)
	}
}

func TestWaitForStableFileReportsMissingFile(t *testing.T) {
	oldTimeout := fileStableTimeout
	oldInterval := fileStableInterval
	fileStableTimeout = 30 * time.Millisecond
	fileStableInterval = 5 * time.Millisecond
	t.Cleanup(func() {
		fileStableTimeout = oldTimeout
		fileStableInterval = oldInterval
	})

	if _, err := waitForStableFile(filepath.Join(t.TempDir(), "missing.mp4")); err == nil {
		t.Fatalf("missing file should fail")
	}
}

func TestMoveRecordingFileReportsDestinationFailure(t *testing.T) {
	oldTimeout := fileStableTimeout
	oldInterval := fileStableInterval
	fileStableTimeout = time.Second
	fileStableInterval = 10 * time.Millisecond
	t.Cleanup(func() {
		fileStableTimeout = oldTimeout
		fileStableInterval = oldInterval
	})

	dir := t.TempDir()
	source := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(source, []byte("clip"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	notDir := filepath.Join(dir, "not-dir")
	if err := os.WriteFile(notDir, []byte("file"), 0o644); err != nil {
		t.Fatalf("write not-dir fixture: %v", err)
	}

	if _, _, err := moveRecordingFile(source, filepath.Join(notDir, "child"), "run"); err == nil {
		t.Fatalf("move should fail when recording folder cannot be created")
	}
}

func TestActiveRecordingForRunSkipsFailedAndMissingRecords(t *testing.T) {
	store := NewMetadataStore(filepath.Join(t.TempDir(), "recordings.json"))
	service := &Service{metadata: store}
	if err := store.Save([]models.RecordingRecord{
		{ID: "failed", RunID: "run-1", Status: models.RecordingStatusFailed},
		{ID: "missing", RunID: "run-1", Status: models.RecordingStatusMissing},
	}); err != nil {
		t.Fatalf("save fixtures: %v", err)
	}

	if _, ok, err := service.activeRecordingForRun("run-1"); err != nil || ok {
		t.Fatalf("failed/missing records should not block duplicate check, ok=%v err=%v", ok, err)
	}

	if err := store.Upsert(models.RecordingRecord{ID: "saved", RunID: "run-1", Status: models.RecordingStatusSaved}); err != nil {
		t.Fatalf("upsert saved fixture: %v", err)
	}
	if record, ok, err := service.activeRecordingForRun("run-1"); err != nil || !ok || record.ID != "saved" {
		t.Fatalf("saved record should block duplicate check, record=%#v ok=%v err=%v", record, ok, err)
	}
}
