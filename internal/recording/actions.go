package recording

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"refleks/internal/models"
)

const missingVideoError = "recording video file is missing"

type commandSpec struct {
	Name string
	Args []string
}

func (s *Service) SetProtected(id string, protected bool) (models.RecordingRecord, error) {
	return s.updateRecording(id, func(record *models.RecordingRecord) error {
		record.Protected = protected
		record.UpdatedAt = nowTimestamp()
		return nil
	})
}

func (s *Service) DeleteRecording(id string) error {
	if s == nil || s.metadata == nil {
		return errors.New("recording service is not initialized")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("recording id is required")
	}

	records, err := s.metadata.List()
	if err != nil {
		return err
	}

	index := -1
	var record models.RecordingRecord
	for i := range records {
		if records[i].ID == id {
			index = i
			record = records[i]
			break
		}
	}
	if index < 0 {
		return fmt.Errorf("recording %q not found", id)
	}

	for _, path := range recordingVideoPaths(record) {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	records = append(records[:index], records[index+1:]...)
	return s.saveRecords(records)
}

func (s *Service) RefreshMissingFiles() ([]models.RecordingRecord, error) {
	if s == nil || s.metadata == nil {
		return []models.RecordingRecord{}, nil
	}

	records, err := s.metadata.List()
	if err != nil {
		return nil, err
	}

	changed := false
	for i := range records {
		next, ok, err := refreshRecordingFileState(records[i])
		if err != nil {
			return nil, err
		}
		if ok {
			records[i] = next
			changed = true
		}
	}
	if changed {
		if err := s.saveRecords(records); err != nil {
			return nil, err
		}
	}
	return records, nil
}

func (s *Service) OpenRecording(id string) error {
	record, err := s.requireExistingVideo(id)
	if err != nil {
		return err
	}
	return openVideoPath(record.VideoPath)
}

func (s *Service) RevealRecording(id string) error {
	record, err := s.requireExistingVideo(id)
	if err != nil {
		return err
	}
	return revealVideoPath(record.VideoPath)
}

func (s *Service) VideoPath(id string) (string, error) {
	record, err := s.requireExistingVideo(id)
	if err != nil {
		return "", err
	}
	return record.VideoPath, nil
}

func (s *Service) requireExistingVideo(id string) (models.RecordingRecord, error) {
	records, err := s.RefreshMissingFiles()
	if err != nil {
		return models.RecordingRecord{}, err
	}
	record, ok := findRecording(records, id)
	if !ok {
		return models.RecordingRecord{}, fmt.Errorf("recording %q not found", strings.TrimSpace(id))
	}
	if strings.TrimSpace(record.VideoPath) == "" {
		return models.RecordingRecord{}, errors.New("recording has no video file path")
	}
	if record.Status == models.RecordingStatusMissing {
		if record.LastError != "" {
			return models.RecordingRecord{}, errors.New(record.LastError)
		}
		return models.RecordingRecord{}, errors.New(missingVideoError)
	}
	return record, nil
}

func (s *Service) updateRecording(id string, mutate func(*models.RecordingRecord) error) (models.RecordingRecord, error) {
	if s == nil || s.metadata == nil {
		return models.RecordingRecord{}, errors.New("recording service is not initialized")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return models.RecordingRecord{}, errors.New("recording id is required")
	}

	records, err := s.metadata.List()
	if err != nil {
		return models.RecordingRecord{}, err
	}

	for i := range records {
		if records[i].ID != id {
			continue
		}
		if err := mutate(&records[i]); err != nil {
			return models.RecordingRecord{}, err
		}
		if err := s.saveRecords(records); err != nil {
			return models.RecordingRecord{}, err
		}
		return records[i], nil
	}
	return models.RecordingRecord{}, fmt.Errorf("recording %q not found", id)
}

func refreshRecordingFileState(record models.RecordingRecord) (models.RecordingRecord, bool, error) {
	path := strings.TrimSpace(record.VideoPath)
	if path == "" {
		return record, false, nil
	}

	info, err := os.Stat(path)
	if err == nil && !info.IsDir() {
		record = refreshAuxiliaryVideoSizes(record, path)
		if record.TrimStatus == models.RecordingTrimStatusPending && samePath(path, record.RawVideoPath) {
			if !pendingTrimIsStale(record, time.Now().UTC()) {
				changed := false
				if record.Status == models.RecordingStatusMissing {
					record.Status = models.RecordingStatusSaved
					changed = true
				}
				if record.SizeBytes != info.Size() {
					record.SizeBytes = info.Size()
					record = syncActiveVideoSize(record, path, info.Size())
					changed = true
				}
				if changed {
					record.UpdatedAt = nowTimestamp()
				}
				return record, changed, nil
			}
			record.TrimStatus = models.RecordingTrimStatusFailed
			record.ActiveVideoKind = models.RecordingVideoKindRaw
			record.LastError = "trim did not complete; using the retained full replay"
			record.SizeBytes = info.Size()
			record = syncActiveVideoSize(record, path, info.Size())
			record.UpdatedAt = nowTimestamp()
			return record, true, nil
		}
		if record.Status == models.RecordingStatusMissing || record.LastError == missingVideoError || record.SizeBytes != info.Size() {
			record.Status = models.RecordingStatusSaved
			record.LastError = ""
			record.SizeBytes = info.Size()
			record = syncActiveVideoSize(record, path, info.Size())
			record.UpdatedAt = nowTimestamp()
			return record, true, nil
		}
		return record, false, nil
	}

	if err == nil && info.IsDir() {
		if record.Status != models.RecordingStatusMissing || record.LastError != "recording video path points to a directory" {
			record.Status = models.RecordingStatusMissing
			record.LastError = "recording video path points to a directory"
			record.SizeBytes = 0
			record.UpdatedAt = nowTimestamp()
			return record, true, nil
		}
		return record, false, nil
	}

	if !errors.Is(err, os.ErrNotExist) {
		return record, false, err
	}
	if fallback, ok := existingRawFallback(record); ok {
		record.VideoPath = fallback.path
		record.SizeBytes = fallback.size
		record.ActiveVideoKind = models.RecordingVideoKindRaw
		record.TrimStatus = models.RecordingTrimStatusFailed
		record.LastError = "trimmed recording video file is missing; falling back to full replay"
		record.UpdatedAt = nowTimestamp()
		return record, true, nil
	}
	if record.Status == models.RecordingStatusSaved || record.Status == models.RecordingStatusPending || record.Status == models.RecordingStatusMissing {
		if record.Status != models.RecordingStatusMissing || record.LastError != missingVideoError || record.SizeBytes != 0 {
			record.Status = models.RecordingStatusMissing
			record.LastError = missingVideoError
			record.SizeBytes = 0
			record.UpdatedAt = nowTimestamp()
			return record, true, nil
		}
	}
	return record, false, nil
}

func pendingTrimIsStale(record models.RecordingRecord, now time.Time) bool {
	startedAt := recordingTimestamp(record.UpdatedAt)
	if startedAt.IsZero() {
		startedAt = recordingTimestamp(record.CreatedAt)
	}
	if startedAt.IsZero() {
		return false
	}
	return now.Sub(startedAt) > stalePendingTrimAfter(record)
}

func stalePendingTrimAfter(record models.RecordingRecord) time.Duration {
	duration := time.Duration(record.ClipDurationMs) * time.Millisecond
	return trimTimeout(duration) + time.Minute
}

func recordingTimestamp(value string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

type rawFallback struct {
	path string
	size int64
}

func existingRawFallback(record models.RecordingRecord) (rawFallback, bool) {
	rawPath := strings.TrimSpace(record.RawVideoPath)
	if rawPath == "" || samePath(rawPath, record.VideoPath) {
		return rawFallback{}, false
	}
	info, err := os.Stat(rawPath)
	if err != nil || info.IsDir() {
		return rawFallback{}, false
	}
	return rawFallback{path: rawPath, size: info.Size()}, true
}

func refreshAuxiliaryVideoSizes(record models.RecordingRecord, activePath string) models.RecordingRecord {
	if path := strings.TrimSpace(record.RawVideoPath); path != "" && !samePath(path, activePath) {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			record.RawSizeBytes = info.Size()
		} else {
			record.RawSizeBytes = 0
		}
	}
	if path := strings.TrimSpace(record.TrimmedVideoPath); path != "" && !samePath(path, activePath) {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			record.TrimmedSizeBytes = info.Size()
		} else {
			record.TrimmedSizeBytes = 0
		}
	}
	return record
}

func syncActiveVideoSize(record models.RecordingRecord, activePath string, size int64) models.RecordingRecord {
	if samePath(activePath, record.RawVideoPath) {
		record.RawSizeBytes = size
	}
	if samePath(activePath, record.TrimmedVideoPath) {
		record.TrimmedSizeBytes = size
	}
	return record
}

func recordingVideoPaths(record models.RecordingRecord) []string {
	seen := map[string]bool{}
	var paths []string
	for _, path := range []string{record.VideoPath, record.RawVideoPath, record.TrimmedVideoPath} {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		key := path
		if abs, err := filepath.Abs(path); err == nil {
			key = strings.ToLower(abs)
		} else {
			key = strings.ToLower(path)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		paths = append(paths, path)
	}
	return paths
}

func findRecording(records []models.RecordingRecord, id string) (models.RecordingRecord, bool) {
	id = strings.TrimSpace(id)
	for _, record := range records {
		if record.ID == id {
			return record, true
		}
	}
	return models.RecordingRecord{}, false
}

func nowTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}
