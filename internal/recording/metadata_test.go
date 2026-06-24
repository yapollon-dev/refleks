package recording

import (
	"path/filepath"
	"testing"
	"time"

	"refleks/internal/models"
)

func TestMetadataStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recordings.json")
	store := NewMetadataStore(path)
	now := time.Now().UTC().Format(time.RFC3339)

	want := models.RecordingRecord{
		ID:          "rec-1",
		RunID:       "run-1",
		RunFileName: "run.refleks",
		RunFilePath: "C:/runs/run.refleks",
		Scenario:    "1w6ts reload v2",
		Score:       1234.5,
		KeepReason:  models.RecordingKeepReasonManual,
		VideoPath:   "C:/clips/run.mp4",
		SizeBytes:   1024,
		Status:      models.RecordingStatusSaved,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := store.Upsert(want); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}

	records, err := store.List()
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("record count = %d, want 1", len(records))
	}
	if records[0].ID != want.ID || records[0].RunID != want.RunID || records[0].VideoPath != want.VideoPath {
		t.Fatalf("round-tripped record mismatch: %#v", records[0])
	}
}

func TestMetadataStoreMissingFileReturnsEmptyList(t *testing.T) {
	store := NewMetadataStore(filepath.Join(t.TempDir(), "recordings.json"))

	records, err := store.List()
	if err != nil {
		t.Fatalf("list missing file failed: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("record count = %d, want 0", len(records))
	}
}

func TestMetadataStoreCompactsLegacySkippedRowsWithoutVideo(t *testing.T) {
	store := NewMetadataStore(filepath.Join(t.TempDir(), "recordings.json"))
	if err := store.Save([]models.RecordingRecord{
		{ID: "skip-empty", Status: models.RecordingStatusSkipped},
		{ID: "skip-with-source", Status: models.RecordingStatusSkipped, OBSSourcePath: "C:/obs/replay.mp4"},
		{ID: "failed", Status: models.RecordingStatusFailed},
		{ID: "saved", Status: models.RecordingStatusSaved, VideoPath: "C:/clips/replay.mp4", SizeBytes: 42},
	}); err != nil {
		t.Fatalf("save fixtures: %v", err)
	}

	if err := store.CompactLegacySkipped(); err != nil {
		t.Fatalf("compact legacy skipped: %v", err)
	}

	records, err := store.List()
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	gotIDs := make(map[string]bool, len(records))
	for _, record := range records {
		gotIDs[record.ID] = true
	}
	if gotIDs["skip-empty"] {
		t.Fatalf("empty legacy skipped row should be removed: %#v", records)
	}
	for _, id := range []string{"skip-with-source", "failed", "saved"} {
		if !gotIDs[id] {
			t.Fatalf("expected row %q to be kept, got %#v", id, records)
		}
	}
}
