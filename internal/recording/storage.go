package recording

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"refleks/internal/models"
)

const bytesPerGB = int64(1024 * 1024 * 1024)

var freeBytesForPath = freeBytesForExistingPath

func (s *Service) PreviewCleanup() (models.RecordingCleanupPreview, error) {
	if s == nil || s.metadata == nil {
		return models.RecordingCleanupPreview{}, nil
	}
	records, err := s.RefreshMissingFiles()
	if err != nil {
		return models.RecordingCleanupPreview{}, err
	}
	return s.cleanupPreview(records, nil)
}

func (s *Service) RunCleanup() (models.RecordingCleanupPreview, error) {
	return s.runCleanup(nil)
}

func (s *Service) runCleanup(excludedIDs map[string]bool) (models.RecordingCleanupPreview, error) {
	if s == nil || s.metadata == nil {
		return models.RecordingCleanupPreview{}, errors.New("recording service is not initialized")
	}

	records, err := s.RefreshMissingFiles()
	if err != nil {
		return models.RecordingCleanupPreview{}, err
	}
	preview, err := s.cleanupPreview(records, excludedIDs)
	if err != nil {
		return preview, err
	}
	if len(preview.Items) == 0 {
		return preview, nil
	}

	deleteIDs := make(map[string]bool, len(preview.Items))
	for _, item := range preview.Items {
		deleteIDs[item.ID] = true
	}

	remaining := records[:0]
	for _, record := range records {
		if !deleteIDs[record.ID] {
			remaining = append(remaining, record)
			continue
		}
		if path := strings.TrimSpace(record.VideoPath); path != "" {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return preview, fmt.Errorf("failed to delete cleanup candidate %q: %w", record.VideoPath, err)
			}
		}
		preview.DeletedCount++
		preview.DeletedBytes += record.SizeBytes
	}
	if err := s.metadata.Save(remaining); err != nil {
		return preview, err
	}
	preview.TotalSizeBytes -= preview.DeletedBytes
	if preview.TotalSizeBytes < 0 {
		preview.TotalSizeBytes = 0
	}
	if preview.RequiredBytes <= preview.DeletedBytes {
		preview.RequiredBytes = 0
		preview.WillMeetLimits = true
		preview.Reason = "Cleanup completed."
	}
	return preview, nil
}

func (s *Service) cleanupPreview(records []models.RecordingRecord, excludedIDs map[string]bool) (models.RecordingCleanupPreview, error) {
	cfg := models.RecordingSettings{}
	if s != nil && s.settingsSvc != nil {
		cfg = s.settingsSvc.Get().Recording
	}
	return buildCleanupPreview(cfg, records, excludedIDs)
}

func buildCleanupPreview(cfg models.RecordingSettings, records []models.RecordingRecord, excludedIDs map[string]bool) (models.RecordingCleanupPreview, error) {
	freeBytes, err := freeBytesForPath(cfg.RecordingDir)
	if err != nil {
		freeBytes = -1
	}
	preview := models.RecordingCleanupPreview{
		TotalSizeBytes:    recordingStorageUsage(records),
		StorageLimitBytes: gbToBytes(cfg.StorageLimitGB),
		MinFreeSpaceBytes: gbToBytes(cfg.MinFreeSpaceGB),
		FreeSpaceBytes:    freeBytes,
	}
	preview.RequiredBytes = requiredCleanupBytes(preview.TotalSizeBytes, preview.StorageLimitBytes, preview.FreeSpaceBytes, preview.MinFreeSpaceBytes)

	candidates := cleanupCandidates(records, excludedIDs, &preview)
	sort.SliceStable(candidates, func(i, j int) bool {
		return cleanupSortTime(candidates[i]).Before(cleanupSortTime(candidates[j]))
	})

	for _, record := range candidates {
		item := cleanupItem(record)
		preview.EligibleCount++
		preview.ReclaimableBytes += item.SizeBytes
		if preview.RequiredBytes > 0 && preview.PlannedDeleteBytes < preview.RequiredBytes {
			preview.Items = append(preview.Items, item)
			preview.PlannedDeleteBytes += item.SizeBytes
			preview.PlannedDeleteCount++
		}
	}

	preview.WillMeetLimits = preview.RequiredBytes == 0 || preview.PlannedDeleteBytes >= preview.RequiredBytes
	if preview.RequiredBytes == 0 {
		preview.Reason = "No cleanup needed."
	} else if preview.WillMeetLimits {
		preview.Reason = "Cleanup can satisfy the configured storage limits."
	} else {
		preview.Reason = "Not enough eligible recordings to satisfy the configured storage limits."
	}
	if err != nil && preview.RequiredBytes == 0 {
		preview.Reason = "Could not check free space; storage-limit cleanup preview is still available."
	}
	return preview, nil
}

func (s *Service) ensureStorageAllowsSave(cfg models.RecordingSettings, incomingBytes int64, excludedID string) error {
	if s == nil || s.metadata == nil {
		return errors.New("recording service is not initialized")
	}
	if strings.TrimSpace(cfg.RecordingDir) == "" {
		return errors.New("recording folder is not configured")
	}

	freeBytes, err := freeBytesForPath(cfg.RecordingDir)
	if err != nil {
		return fmt.Errorf("could not check free space for recording folder: %w", err)
	}
	minFreeBytes := gbToBytes(cfg.MinFreeSpaceGB)
	if minFreeBytes > 0 && freeBytes-incomingBytes < minFreeBytes {
		return fmt.Errorf(
			"insufficient disk space: saving this replay would leave %s free, below the configured minimum of %s",
			formatStorageBytes(freeBytes-incomingBytes),
			formatStorageBytes(minFreeBytes),
		)
	}

	records, err := s.List()
	if err != nil {
		return err
	}
	totalAfterSave := recordingStorageUsage(records) + incomingBytes
	limitBytes := gbToBytes(cfg.StorageLimitGB)
	if limitBytes <= 0 || totalAfterSave <= limitBytes {
		return nil
	}

	overage := totalAfterSave - limitBytes
	if !cfg.AutoCleanup {
		return fmt.Errorf(
			"recording storage limit exceeded: saving this replay would use %s of the configured %s limit",
			formatStorageBytes(totalAfterSave),
			formatStorageBytes(limitBytes),
		)
	}

	excludedIDs := map[string]bool{}
	if excludedID != "" {
		excludedIDs[excludedID] = true
	}
	preview := models.RecordingCleanupPreview{}
	candidates := cleanupCandidates(records, excludedIDs, &preview)
	var reclaimable int64
	for _, candidate := range candidates {
		reclaimable += candidate.SizeBytes
	}
	if reclaimable < overage {
		return fmt.Errorf(
			"recording storage limit exceeded: auto-cleanup can reclaim %s, but %s is required",
			formatStorageBytes(reclaimable),
			formatStorageBytes(overage),
		)
	}
	return nil
}

func cleanupCandidates(records []models.RecordingRecord, excludedIDs map[string]bool, preview *models.RecordingCleanupPreview) []models.RecordingRecord {
	var candidates []models.RecordingRecord
	for _, record := range records {
		if excludedIDs != nil && excludedIDs[record.ID] {
			continue
		}
		if record.Status != models.RecordingStatusSaved || strings.TrimSpace(record.VideoPath) == "" || record.SizeBytes <= 0 {
			preview.ExcludedUnavailableCount++
			continue
		}
		if record.Protected {
			preview.ExcludedProtectedCount++
			continue
		}
		if record.PBAtSave {
			preview.ExcludedPBCount++
			continue
		}
		candidates = append(candidates, record)
	}
	return candidates
}

func cleanupItem(record models.RecordingRecord) models.RecordingCleanupItem {
	return models.RecordingCleanupItem{
		ID:         record.ID,
		Scenario:   record.Scenario,
		VideoPath:  record.VideoPath,
		SizeBytes:  record.SizeBytes,
		KeepReason: record.KeepReason,
		CreatedAt:  record.CreatedAt,
		Protected:  record.Protected,
		PBAtSave:   record.PBAtSave,
	}
}

func cleanupSortTime(record models.RecordingRecord) time.Time {
	for _, value := range []string{record.CreatedAt, record.PlayedAt, record.UpdatedAt} {
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func recordingStorageUsage(records []models.RecordingRecord) int64 {
	var total int64
	for _, record := range records {
		if record.SizeBytes > 0 {
			total += record.SizeBytes
		}
	}
	return total
}

func requiredCleanupBytes(totalSize, storageLimit, freeBytes, minFreeBytes int64) int64 {
	var required int64
	if storageLimit > 0 && totalSize > storageLimit {
		required = totalSize - storageLimit
	}
	if minFreeBytes > 0 && freeBytes >= 0 && freeBytes < minFreeBytes {
		if need := minFreeBytes - freeBytes; need > required {
			required = need
		}
	}
	return required
}

func gbToBytes(gb int) int64 {
	if gb <= 0 {
		return 0
	}
	return int64(gb) * bytesPerGB
}

func freeBytesForExistingPath(path string) (int64, error) {
	target := strings.TrimSpace(path)
	if target == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return 0, err
		}
		target = cwd
	}
	target = filepath.Clean(target)
	for {
		if _, err := os.Stat(target); err == nil {
			return platformFreeBytes(target)
		}
		parent := filepath.Dir(target)
		if parent == target || parent == "." {
			break
		}
		target = parent
	}
	cwd, err := os.Getwd()
	if err != nil {
		return 0, err
	}
	return platformFreeBytes(cwd)
}

func formatStorageBytes(bytes int64) string {
	if bytes <= 0 {
		return "0 B"
	}
	const unit = float64(1024)
	value := float64(bytes)
	units := []string{"B", "KB", "MB", "GB", "TB"}
	unitIndex := 0
	for value >= unit && unitIndex < len(units)-1 {
		value /= unit
		unitIndex++
	}
	if unitIndex == 0 {
		return fmt.Sprintf("%d %s", bytes, units[unitIndex])
	}
	return fmt.Sprintf("%.1f %s", value, units[unitIndex])
}
