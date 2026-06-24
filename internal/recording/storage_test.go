package recording

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"refleks/internal/models"
	"refleks/internal/runs"
	appsettings "refleks/internal/settings"
)

func withFakeFreeBytes(t *testing.T, freeBytes int64) {
	t.Helper()
	previous := freeBytesForPath
	freeBytesForPath = func(string) (int64, error) {
		return freeBytes, nil
	}
	t.Cleanup(func() {
		freeBytesForPath = previous
	})
}

func newStorageTestService(t *testing.T, cfg models.RecordingSettings) *Service {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	settingsSvc := appsettings.NewService()
	settings := settingsSvc.Get()
	settings.Recording = cfg
	if err := settingsSvc.Update(settings); err != nil {
		t.Fatalf("update settings: %v", err)
	}
	return &Service{
		metadata:    NewMetadataStore(filepath.Join(t.TempDir(), "recordings.json")),
		settingsSvc: settingsSvc,
		runStore:    runs.NewStore(settingsSvc),
	}
}

func TestCleanupPreviewExcludesProtectedAndPBRecordings(t *testing.T) {
	withFakeFreeBytes(t, 10*bytesPerGB)
	cfg := models.RecordingSettings{
		RecordingDir:   t.TempDir(),
		StorageLimitGB: 1,
		MinFreeSpaceGB: 1,
		AutoCleanup:    true,
	}
	records := []models.RecordingRecord{
		{ID: "old", Status: models.RecordingStatusSaved, VideoPath: "old.mp4", SizeBytes: bytesPerGB, CreatedAt: "2026-06-19T01:00:00Z"},
		{ID: "protected", Status: models.RecordingStatusSaved, VideoPath: "protected.mp4", SizeBytes: bytesPerGB, Protected: true},
		{ID: "pb", Status: models.RecordingStatusSaved, VideoPath: "pb.mp4", SizeBytes: bytesPerGB, PBAtSave: true},
		{ID: "missing", Status: models.RecordingStatusMissing, VideoPath: "missing.mp4", SizeBytes: bytesPerGB},
	}

	preview, err := buildCleanupPreview(cfg, records, nil)
	if err != nil {
		t.Fatalf("preview cleanup: %v", err)
	}
	if preview.ExcludedProtectedCount != 1 || preview.ExcludedPBCount != 1 || preview.ExcludedUnavailableCount != 1 {
		t.Fatalf("unexpected exclusions: %#v", preview)
	}
	if preview.PlannedDeleteCount != 1 || len(preview.Items) != 1 || preview.Items[0].ID != "old" {
		t.Fatalf("unexpected cleanup candidates: %#v", preview.Items)
	}
	if preview.WillMeetLimits {
		t.Fatalf("preview should not meet limit with protected/PB recordings excluded: %#v", preview)
	}
}

func TestEnsureStorageAllowsSaveRespectsFreeSpaceProtection(t *testing.T) {
	withFakeFreeBytes(t, 5*bytesPerGB)
	cfg := models.RecordingSettings{
		RecordingDir:   t.TempDir(),
		StorageLimitGB: 25,
		MinFreeSpaceGB: 5,
	}
	service := newStorageTestService(t, cfg)

	err := service.ensureStorageAllowsSave(cfg, bytesPerGB, "new")
	if err == nil {
		t.Fatalf("expected free-space protection error")
	}
	if !strings.Contains(err.Error(), "insufficient disk space") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureStorageAllowsSaveUsesAutoCleanupCapacity(t *testing.T) {
	withFakeFreeBytes(t, 50*bytesPerGB)
	cfg := models.RecordingSettings{
		RecordingDir:   t.TempDir(),
		StorageLimitGB: 25,
		MinFreeSpaceGB: 1,
		AutoCleanup:    true,
	}
	service := newStorageTestService(t, cfg)
	if err := service.metadata.Save([]models.RecordingRecord{
		{ID: "old", Status: models.RecordingStatusSaved, VideoPath: "old.mp4", SizeBytes: 2 * bytesPerGB},
		{ID: "protected", Status: models.RecordingStatusSaved, VideoPath: "protected.mp4", SizeBytes: 23 * bytesPerGB, Protected: true},
	}); err != nil {
		t.Fatalf("save fixtures: %v", err)
	}

	if err := service.ensureStorageAllowsSave(cfg, bytesPerGB, "new"); err != nil {
		t.Fatalf("auto cleanup should make storage guard pass: %v", err)
	}

	cfg.AutoCleanup = false
	if err := service.ensureStorageAllowsSave(cfg, bytesPerGB, "new"); err == nil {
		t.Fatalf("storage guard should fail without auto cleanup")
	}
}

func TestRecordingStorageUsageCountsOnlySavedVideos(t *testing.T) {
	records := []models.RecordingRecord{
		{ID: "saved", Status: models.RecordingStatusSaved, VideoPath: "saved.mp4", SizeBytes: 10},
		{ID: "with-raw", Status: models.RecordingStatusSaved, VideoPath: "clip.mp4", SizeBytes: 7, RawVideoPath: "full.mp4", RawSizeBytes: 13, TrimmedVideoPath: "clip.mp4", TrimmedSizeBytes: 7},
		{ID: "failed", Status: models.RecordingStatusFailed, VideoPath: "failed.mp4", SizeBytes: 20},
		{ID: "missing", Status: models.RecordingStatusMissing, VideoPath: "missing.mp4", SizeBytes: 30},
		{ID: "skipped", Status: models.RecordingStatusSkipped, SizeBytes: 40},
		{ID: "no-path", Status: models.RecordingStatusSaved, SizeBytes: 50},
	}

	if got := recordingStorageUsage(records); got != 30 {
		t.Fatalf("storage usage = %d, want 30", got)
	}
}

func TestLocalStatusCountsOnlySavedVideos(t *testing.T) {
	cfg := models.RecordingSettings{RecordingDir: t.TempDir()}
	service := newStorageTestService(t, cfg)
	if err := service.metadata.Save([]models.RecordingRecord{
		{ID: "saved", Status: models.RecordingStatusSaved, VideoPath: "saved.mp4", SizeBytes: 10},
		{ID: "with-raw", Status: models.RecordingStatusSaved, VideoPath: "clip.mp4", SizeBytes: 7, RawVideoPath: "full.mp4", RawSizeBytes: 13, TrimmedVideoPath: "clip.mp4", TrimmedSizeBytes: 7},
		{ID: "failed", Status: models.RecordingStatusFailed, VideoPath: "failed.mp4", SizeBytes: 20},
		{ID: "skipped", Status: models.RecordingStatusSkipped, SizeBytes: 30},
	}); err != nil {
		t.Fatalf("save fixtures: %v", err)
	}

	status := service.localStatus()
	if status.TotalRecordings != 2 || status.TotalSizeBytes != 30 {
		t.Fatalf("status totals should count saved videos only, got %#v", status)
	}
}

func TestRunCleanupDeletesEligibleRecordingsOnly(t *testing.T) {
	withFakeFreeBytes(t, bytesPerGB-1)
	cfg := models.RecordingSettings{
		RecordingDir:   t.TempDir(),
		StorageLimitGB: 25,
		MinFreeSpaceGB: 1,
		AutoCleanup:    true,
	}
	service := newStorageTestService(t, cfg)
	dir := t.TempDir()
	eligiblePath := filepath.Join(dir, "eligible.mp4")
	protectedPath := filepath.Join(dir, "protected.mp4")
	if err := os.WriteFile(eligiblePath, []byte("eligible"), 0o644); err != nil {
		t.Fatalf("write eligible video: %v", err)
	}
	if err := os.WriteFile(protectedPath, []byte("protected"), 0o644); err != nil {
		t.Fatalf("write protected video: %v", err)
	}
	if err := service.metadata.Save([]models.RecordingRecord{
		{ID: "eligible", Status: models.RecordingStatusSaved, VideoPath: eligiblePath, SizeBytes: int64(len("eligible"))},
		{ID: "protected", Status: models.RecordingStatusSaved, VideoPath: protectedPath, SizeBytes: int64(len("protected")), Protected: true},
	}); err != nil {
		t.Fatalf("save fixtures: %v", err)
	}

	preview, err := service.RunCleanup()
	if err != nil {
		t.Fatalf("run cleanup: %v", err)
	}
	if preview.DeletedCount != 1 || preview.DeletedBytes != int64(len("eligible")) {
		t.Fatalf("unexpected cleanup result: %#v", preview)
	}
	if _, err := os.Stat(eligiblePath); !os.IsNotExist(err) {
		t.Fatalf("eligible file should be deleted, stat error = %v", err)
	}
	if _, err := os.Stat(protectedPath); err != nil {
		t.Fatalf("protected file should remain: %v", err)
	}
	records, err := service.metadata.List()
	if err != nil {
		t.Fatalf("list metadata: %v", err)
	}
	if len(records) != 1 || records[0].ID != "protected" {
		t.Fatalf("cleanup should remove only eligible metadata: %#v", records)
	}
}
