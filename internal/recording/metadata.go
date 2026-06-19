package recording

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"refleks/internal/models"
)

// MetadataStore persists run-linked recording metadata as a local JSON file.
type MetadataStore struct {
	mu   sync.Mutex
	path string
}

func NewMetadataStore(path string) *MetadataStore {
	return &MetadataStore{path: path}
}

func (s *MetadataStore) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *MetadataStore) List() ([]models.RecordingRecord, error) {
	if s == nil {
		return nil, errors.New("recording metadata store is not initialized")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *MetadataStore) Save(records []models.RecordingRecord) error {
	if s == nil {
		return errors.New("recording metadata store is not initialized")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(records)
}

func (s *MetadataStore) Upsert(record models.RecordingRecord) error {
	if s == nil {
		return errors.New("recording metadata store is not initialized")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	records, err := s.loadLocked()
	if err != nil {
		return err
	}

	replaced := false
	for i := range records {
		if records[i].ID == record.ID {
			records[i] = record
			replaced = true
			break
		}
	}
	if !replaced {
		records = append(records, record)
	}
	return s.saveLocked(records)
}

func (s *MetadataStore) loadLocked() ([]models.RecordingRecord, error) {
	if _, err := os.Stat(s.path); errors.Is(err, os.ErrNotExist) {
		return []models.RecordingRecord{}, nil
	} else if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return []models.RecordingRecord{}, nil
	}

	var records []models.RecordingRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	if records == nil {
		records = []models.RecordingRecord{}
	}
	return records, nil
}

func (s *MetadataStore) saveLocked(records []models.RecordingRecord) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp := s.path + ".tmp"
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
