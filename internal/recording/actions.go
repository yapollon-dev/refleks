package recording

import (
	"errors"
	"fmt"
	"os"
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

	if path := strings.TrimSpace(record.VideoPath); path != "" {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	records = append(records[:index], records[index+1:]...)
	return s.metadata.Save(records)
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
		if err := s.metadata.Save(records); err != nil {
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
		if err := s.metadata.Save(records); err != nil {
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
		if record.Status == models.RecordingStatusMissing || record.LastError == missingVideoError || record.SizeBytes != info.Size() {
			record.Status = models.RecordingStatusSaved
			record.LastError = ""
			record.SizeBytes = info.Size()
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
