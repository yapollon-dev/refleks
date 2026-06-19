package recording

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"refleks/internal/constants"
	"refleks/internal/models"
	appsettings "refleks/internal/settings"
)

type Service struct {
	mu          sync.RWMutex
	settingsSvc *appsettings.Service
	metadata    *MetadataStore
	lastStatus  models.RecordingRuntimeStatus
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
	status := s.localStatus()
	if s == nil {
		return status
	}

	s.mu.RLock()
	last := s.lastStatus
	s.mu.RUnlock()

	if last.ConnectionStatus != "" {
		status.ConnectionStatus = last.ConnectionStatus
		status.ReplayBufferStatus = last.ReplayBufferStatus
		status.OBSVersion = last.OBSVersion
		status.OBSWebSocketVersion = last.OBSWebSocketVersion
		status.LastError = last.LastError
	}
	return status
}

func (s *Service) TestConnection(ctx context.Context) models.RecordingRuntimeStatus {
	status := s.localStatus()
	if s == nil || s.settingsSvc == nil {
		status.ConnectionStatus = "error"
		status.ReplayBufferStatus = "unknown"
		status.LastError = "recording service is not initialized"
		return status
	}

	cfg := s.settingsSvc.Get().Recording
	client := NewOBSClient(cfg)
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		status.ConnectionStatus = classifyConnectionError(err)
		status.ReplayBufferStatus = "unknown"
		status.LastError = err.Error()
		s.cacheStatus(status)
		return status
	}
	defer client.Close()

	status.ConnectionStatus = "connected"
	version, err := client.GetVersion(ctx)
	if err != nil {
		status.ConnectionStatus = "error"
		status.ReplayBufferStatus = "unknown"
		status.LastError = err.Error()
		s.cacheStatus(status)
		return status
	}
	status.OBSVersion = version.OBSVersion
	status.OBSWebSocketVersion = version.OBSWebSocketVersion

	replay, err := client.GetReplayBufferStatus(ctx)
	if err != nil {
		status.ReplayBufferStatus = "unknown"
		status.LastError = err.Error()
		s.cacheStatus(status)
		return status
	}
	if replay.Active {
		status.ReplayBufferStatus = "active"
	} else {
		status.ReplayBufferStatus = "inactive"
	}
	status.LastError = ""
	s.cacheStatus(status)
	return status
}

func (s *Service) localStatus() models.RecordingRuntimeStatus {
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

func (s *Service) cacheStatus(status models.RecordingRuntimeStatus) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastStatus = status
}

func classifyConnectionError(err error) string {
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "authentication") || strings.Contains(msg, "password") {
		return "auth_failed"
	}
	return "error"
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
