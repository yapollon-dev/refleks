package recording

import (
	"path/filepath"

	"refleks/internal/constants"
	"refleks/internal/models"
	appsettings "refleks/internal/settings"
)

type Service struct {
	settingsSvc *appsettings.Service
	metadata    *MetadataStore
}

func NewService(settingsSvc *appsettings.Service) (*Service, error) {
	path, err := MetadataPath()
	if err != nil {
		return nil, err
	}
	return &Service{
		settingsSvc: settingsSvc,
		metadata:    NewMetadataStore(path),
	}, nil
}

func MetadataPath() (string, error) {
	base, err := appsettings.GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, constants.RecordingsFileName), nil
}

func (s *Service) Status() models.RecordingRuntimeStatus {
	cfg := models.RecordingSettings{}
	if s != nil && s.settingsSvc != nil {
		cfg = s.settingsSvc.Get().Recording
	}

	records, err := s.List()
	status := models.RecordingRuntimeStatus{
		Enabled:            cfg.Enabled,
		RecordingDir:       cfg.RecordingDir,
		ConnectionStatus:   "not_configured",
		ReplayBufferStatus: "not_checked",
	}
	if s != nil && s.metadata != nil {
		status.MetadataPath = s.metadata.Path()
	}
	if err != nil {
		status.LastError = err.Error()
		return status
	}
	status.TotalRecordings = len(records)
	for _, record := range records {
		status.TotalSizeBytes += record.SizeBytes
	}
	if cfg.Enabled {
		status.ConnectionStatus = "not_checked"
	}
	return status
}

func (s *Service) List() ([]models.RecordingRecord, error) {
	if s == nil || s.metadata == nil {
		return []models.RecordingRecord{}, nil
	}
	return s.metadata.List()
}

func (s *Service) RecordingDir() string {
	if s == nil || s.settingsSvc == nil {
		return ""
	}
	return s.settingsSvc.Get().Recording.RecordingDir
}
