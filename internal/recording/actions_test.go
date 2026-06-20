package recording

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"refleks/internal/models"
)

func TestSetProtectedUpdatesFlag(t *testing.T) {
	store := NewMetadataStore(filepath.Join(t.TempDir(), "recordings.json"))
	service := &Service{metadata: store}
	record := models.RecordingRecord{ID: "recording-1", Status: models.RecordingStatusSaved}
	if err := store.Save([]models.RecordingRecord{record}); err != nil {
		t.Fatalf("save fixture: %v", err)
	}

	protected, err := service.SetProtected(record.ID, true)
	if err != nil {
		t.Fatalf("protect recording: %v", err)
	}
	if !protected.Protected || protected.UpdatedAt == "" {
		t.Fatalf("protected record not updated: %#v", protected)
	}

	unprotected, err := service.SetProtected(record.ID, false)
	if err != nil {
		t.Fatalf("unprotect recording: %v", err)
	}
	if unprotected.Protected {
		t.Fatalf("record should be unprotected: %#v", unprotected)
	}
}

func TestDeleteRecordingRemovesMetadataAndVideoOnly(t *testing.T) {
	dir := t.TempDir()
	store := NewMetadataStore(filepath.Join(dir, "recordings.json"))
	service := &Service{metadata: store}

	videoPath := filepath.Join(dir, "clip.mp4")
	runPath := filepath.Join(dir, "run.refleks")
	if err := os.WriteFile(videoPath, []byte("video"), 0o644); err != nil {
		t.Fatalf("write video fixture: %v", err)
	}
	if err := os.WriteFile(runPath, []byte("run"), 0o644); err != nil {
		t.Fatalf("write run fixture: %v", err)
	}
	record := models.RecordingRecord{
		ID:          "recording-1",
		RunFilePath: runPath,
		VideoPath:   videoPath,
		Status:      models.RecordingStatusSaved,
	}
	if err := store.Save([]models.RecordingRecord{record}); err != nil {
		t.Fatalf("save fixture: %v", err)
	}

	if err := service.DeleteRecording(record.ID); err != nil {
		t.Fatalf("delete recording: %v", err)
	}
	if _, err := os.Stat(videoPath); !os.IsNotExist(err) {
		t.Fatalf("video should be deleted, stat error = %v", err)
	}
	if _, err := os.Stat(runPath); err != nil {
		t.Fatalf("linked run should remain: %v", err)
	}
	records, err := store.List()
	if err != nil {
		t.Fatalf("list records: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("metadata should be empty after delete: %#v", records)
	}
}

func TestDeleteRecordingRemovesRawAndTrimmedVideos(t *testing.T) {
	dir := t.TempDir()
	store := NewMetadataStore(filepath.Join(dir, "recordings.json"))
	service := &Service{metadata: store}

	rawPath := filepath.Join(dir, "full replay.mp4")
	trimmedPath := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(rawPath, []byte("raw"), 0o644); err != nil {
		t.Fatalf("write raw fixture: %v", err)
	}
	if err := os.WriteFile(trimmedPath, []byte("trimmed"), 0o644); err != nil {
		t.Fatalf("write trimmed fixture: %v", err)
	}
	record := models.RecordingRecord{
		ID:               "recording-1",
		VideoPath:        trimmedPath,
		RawVideoPath:     rawPath,
		TrimmedVideoPath: trimmedPath,
		Status:           models.RecordingStatusSaved,
	}
	if err := store.Save([]models.RecordingRecord{record}); err != nil {
		t.Fatalf("save fixture: %v", err)
	}

	if err := service.DeleteRecording(record.ID); err != nil {
		t.Fatalf("delete recording: %v", err)
	}
	for _, path := range []string{rawPath, trimmedPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s should be deleted, stat error = %v", path, err)
		}
	}
}

func TestRefreshMissingFilesDetectsMissingAndRecovery(t *testing.T) {
	dir := t.TempDir()
	store := NewMetadataStore(filepath.Join(dir, "recordings.json"))
	service := &Service{metadata: store}
	videoPath := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(videoPath, []byte("video"), 0o644); err != nil {
		t.Fatalf("write video fixture: %v", err)
	}
	record := models.RecordingRecord{
		ID:        "recording-1",
		VideoPath: videoPath,
		SizeBytes: 5,
		Status:    models.RecordingStatusSaved,
	}
	if err := store.Save([]models.RecordingRecord{record}); err != nil {
		t.Fatalf("save fixture: %v", err)
	}
	if err := os.Remove(videoPath); err != nil {
		t.Fatalf("remove video fixture: %v", err)
	}

	missing, err := service.RefreshMissingFiles()
	if err != nil {
		t.Fatalf("refresh missing files: %v", err)
	}
	if len(missing) != 1 || missing[0].Status != models.RecordingStatusMissing || missing[0].LastError != missingVideoError || missing[0].SizeBytes != 0 {
		t.Fatalf("missing file not reflected in metadata: %#v", missing)
	}

	if err := os.WriteFile(videoPath, []byte("restored"), 0o644); err != nil {
		t.Fatalf("restore video fixture: %v", err)
	}
	recovered, err := service.RefreshMissingFiles()
	if err != nil {
		t.Fatalf("refresh recovered file: %v", err)
	}
	if len(recovered) != 1 || recovered[0].Status != models.RecordingStatusSaved || recovered[0].LastError != "" || recovered[0].SizeBytes != int64(len("restored")) {
		t.Fatalf("recovered file not reflected in metadata: %#v", recovered)
	}
}

func TestRefreshMissingFilesKeepsRecentPendingTrimAsRawFallback(t *testing.T) {
	dir := t.TempDir()
	store := NewMetadataStore(filepath.Join(dir, "recordings.json"))
	service := &Service{metadata: store}
	rawPath := filepath.Join(dir, "full replay.mp4")
	if err := os.WriteFile(rawPath, []byte("raw-video"), 0o644); err != nil {
		t.Fatalf("write raw fixture: %v", err)
	}
	record := models.RecordingRecord{
		ID:              "recording-1",
		VideoPath:       rawPath,
		RawVideoPath:    rawPath,
		RawSizeBytes:    9,
		Status:          models.RecordingStatusSaved,
		ActiveVideoKind: models.RecordingVideoKindRaw,
		TrimStatus:      models.RecordingTrimStatusPending,
		UpdatedAt:       nowTimestamp(),
	}
	if err := store.Save([]models.RecordingRecord{record}); err != nil {
		t.Fatalf("save fixture: %v", err)
	}

	records, err := service.RefreshMissingFiles()
	if err != nil {
		t.Fatalf("refresh recordings: %v", err)
	}
	if len(records) != 1 || records[0].TrimStatus != models.RecordingTrimStatusPending || records[0].ActiveVideoKind != models.RecordingVideoKindRaw {
		t.Fatalf("recent pending trim should stay pending: %#v", records)
	}
	if records[0].LastError != "" || records[0].Status != models.RecordingStatusSaved {
		t.Fatalf("recent pending trim should not surface a failure: %#v", records[0])
	}
}

func TestRefreshMissingFilesRecoversStalePendingTrimAsRawFallback(t *testing.T) {
	dir := t.TempDir()
	store := NewMetadataStore(filepath.Join(dir, "recordings.json"))
	service := &Service{metadata: store}
	rawPath := filepath.Join(dir, "full replay.mp4")
	if err := os.WriteFile(rawPath, []byte("raw-video"), 0o644); err != nil {
		t.Fatalf("write raw fixture: %v", err)
	}
	record := models.RecordingRecord{
		ID:              "recording-1",
		VideoPath:       rawPath,
		RawVideoPath:    rawPath,
		RawSizeBytes:    9,
		Status:          models.RecordingStatusSaved,
		ActiveVideoKind: models.RecordingVideoKindRaw,
		TrimStatus:      models.RecordingTrimStatusPending,
		UpdatedAt:       time.Now().UTC().Add(-30 * time.Minute).Format(time.RFC3339),
	}
	if err := store.Save([]models.RecordingRecord{record}); err != nil {
		t.Fatalf("save fixture: %v", err)
	}

	records, err := service.RefreshMissingFiles()
	if err != nil {
		t.Fatalf("refresh recordings: %v", err)
	}
	if len(records) != 1 || records[0].TrimStatus != models.RecordingTrimStatusFailed || records[0].ActiveVideoKind != models.RecordingVideoKindRaw {
		t.Fatalf("stale pending trim should become raw fallback: %#v", records)
	}
	if records[0].LastError == "" || records[0].Status != models.RecordingStatusSaved {
		t.Fatalf("stale pending trim should keep saved raw video with an error: %#v", records[0])
	}
}
