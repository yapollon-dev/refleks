package recording

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"refleks/internal/constants"
	"refleks/internal/models"
	"refleks/internal/runs"
	appsettings "refleks/internal/settings"
)

type Service struct {
	mu          sync.RWMutex
	saveMu      sync.Mutex
	settingsSvc *appsettings.Service
	metadata    *MetadataStore
	runStore    *runs.Store
	lastStatus  models.RecordingRuntimeStatus
}

func NewService(settingsSvc *appsettings.Service, runStore *runs.Store) (*Service, error) {
	path, err := MetadataPath()
	if err != nil {
		return nil, err
	}
	return &Service{
		settingsSvc: settingsSvc,
		metadata:    NewMetadataStore(path),
		runStore:    runStore,
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
	if !replay.Active && cfg.Enabled && cfg.AutoConnect && cfg.AutoStartReplayBuffer {
		if err := client.StartReplayBuffer(ctx); err != nil {
			status.ReplayBufferStatus = "inactive"
			status.LastError = "failed to start OBS replay buffer: " + err.Error()
			s.cacheStatus(status)
			return status
		}
		status.ReplayBufferStatus = "active"
	}
	status.LastError = ""
	s.cacheStatus(status)
	return status
}

func (s *Service) EnsureReplayBufferStarted(ctx context.Context) models.RecordingRuntimeStatus {
	status := s.localStatus()
	if s == nil || s.settingsSvc == nil {
		status.ConnectionStatus = "error"
		status.ReplayBufferStatus = "unknown"
		status.LastError = "recording service is not initialized"
		return status
	}

	cfg := s.settingsSvc.Get().Recording
	if !cfg.Enabled || !cfg.AutoConnect || !cfg.AutoStartReplayBuffer {
		return status
	}

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
	if err := s.ensureReplayBuffer(ctx, client, cfg); err != nil {
		status.ReplayBufferStatus = "inactive"
		status.LastError = err.Error()
		s.cacheStatus(status)
		return status
	}
	status.ReplayBufferStatus = "active"
	status.LastError = ""
	s.cacheStatus(status)
	return status
}

func (s *Service) SaveLatestRunReplay(ctx context.Context) (models.RecordingRecord, error) {
	if s == nil || s.runStore == nil {
		return models.RecordingRecord{}, errors.New("run store is not initialized")
	}
	recent, err := s.runStore.LoadRecentRuns(1)
	if err != nil {
		return models.RecordingRecord{}, err
	}
	if len(recent) == 0 {
		return models.RecordingRecord{}, errors.New("no completed runs are available")
	}
	return s.SaveRunReplay(ctx, recent[len(recent)-1])
}

func (s *Service) SaveRunReplay(ctx context.Context, rec models.RunRecord) (models.RecordingRecord, error) {
	return s.saveRunReplay(ctx, rec, models.RecordingKeepReasonManual, false)
}

func (s *Service) HandleCompletedRun(ctx context.Context, rec models.RunRecord) (models.RecordingRecord, error) {
	if s == nil || s.settingsSvc == nil || s.runStore == nil {
		return models.RecordingRecord{}, errors.New("recording service is not initialized")
	}

	cfg := s.settingsSvc.Get().Recording
	if !cfg.Enabled {
		return models.RecordingRecord{}, nil
	}
	rec = runs.EnsureRunID(rec)
	if exists, err := s.autoMetadataExistsForRun(rec.RunID); err != nil {
		return models.RecordingRecord{}, err
	} else if exists {
		return models.RecordingRecord{}, nil
	}

	allRuns, err := s.runStore.LoadAllRunSummaries()
	if err != nil {
		return models.RecordingRecord{}, err
	}
	decision := EvaluatePolicy(cfg, rec, allRuns)
	if !decision.ShouldSave {
		record := newRecordingRecord(rec, decision.Reason, decision.PBAtSave)
		record.Status = models.RecordingStatusSkipped
		record.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if err := s.metadata.Upsert(record); err != nil {
			return models.RecordingRecord{}, err
		}
		return record, nil
	}
	if !cfg.AutoConnect {
		record := newRecordingRecord(rec, decision.Reason, decision.PBAtSave)
		record.Status = models.RecordingStatusFailed
		record.LastError = "automatic OBS connection is disabled"
		record.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		if err := s.metadata.Upsert(record); err != nil {
			return models.RecordingRecord{}, err
		}
		return record, errors.New(record.LastError)
	}
	return s.saveRunReplay(ctx, rec, decision.Reason, decision.PBAtSave)
}

func (s *Service) saveRunReplay(ctx context.Context, rec models.RunRecord, reason models.RecordingKeepReason, pbAtSave bool) (models.RecordingRecord, error) {
	if s == nil || s.settingsSvc == nil || s.metadata == nil {
		return models.RecordingRecord{}, errors.New("recording service is not initialized")
	}

	s.saveMu.Lock()
	defer s.saveMu.Unlock()

	cfg := s.settingsSvc.Get().Recording
	if !cfg.Enabled {
		return models.RecordingRecord{}, errors.New("recording is disabled")
	}
	if strings.TrimSpace(cfg.RecordingDir) == "" {
		return models.RecordingRecord{}, errors.New("recording folder is not configured")
	}

	rec = runs.EnsureRunID(rec)
	if existing, ok, err := s.activeRecordingForRun(rec.RunID); err != nil {
		return models.RecordingRecord{}, err
	} else if ok {
		return existing, fmt.Errorf("recording already exists for this run")
	}

	record := newRecordingRecord(rec, reason, pbAtSave)
	if err := s.metadata.Upsert(record); err != nil {
		return models.RecordingRecord{}, err
	}

	fail := func(err error) (models.RecordingRecord, error) {
		record.Status = models.RecordingStatusFailed
		record.LastError = err.Error()
		record.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		_ = s.metadata.Upsert(record)
		return record, err
	}

	if err := s.ensureStorageAllowsSave(cfg, 0, record.ID); err != nil {
		return fail(err)
	}

	client := NewOBSClient(cfg)
	obsCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := client.Connect(obsCtx); err != nil {
		return fail(describeOBSConnectionError(err))
	}
	defer client.Close()

	if err := s.ensureReplayBuffer(obsCtx, client, cfg); err != nil {
		return fail(err)
	}

	sourcePath, err := client.SaveReplayBuffer(obsCtx)
	if err != nil {
		return fail(fmt.Errorf("OBS replay buffer save failed: %w", err))
	}
	record.OBSSourcePath = sourcePath
	if err := s.metadata.Upsert(record); err != nil {
		return fail(err)
	}

	finalPath, size, err := moveRecordingFileWithCheck(sourcePath, cfg.RecordingDir, rec.FileName, func(size int64) error {
		return s.ensureStorageAllowsSave(cfg, size, record.ID)
	})
	if err != nil {
		return fail(err)
	}

	record.VideoPath = finalPath
	record.SizeBytes = size
	record.Status = models.RecordingStatusSaved
	record.LastError = ""
	record.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := s.metadata.Upsert(record); err != nil {
		return record, err
	}
	if cfg.AutoCleanup {
		if _, err := s.runCleanup(map[string]bool{record.ID: true}); err != nil {
			record.LastError = "recording saved, but automatic cleanup failed: " + err.Error()
			record.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			_ = s.metadata.Upsert(record)
		}
	}
	return record, nil
}

func (s *Service) ensureReplayBuffer(ctx context.Context, client *OBSClient, cfg models.RecordingSettings) error {
	status, err := client.GetReplayBufferStatus(ctx)
	if err != nil {
		return fmt.Errorf("failed to read OBS replay buffer status: %w", err)
	}
	if status.Active {
		return nil
	}
	if !cfg.AutoStartReplayBuffer {
		return errors.New("OBS replay buffer is inactive and auto-start is disabled")
	}
	if err := client.StartReplayBuffer(ctx); err != nil {
		return fmt.Errorf("failed to start OBS replay buffer: %w", err)
	}
	return nil
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
	status.StorageLimitBytes = gbToBytes(cfg.StorageLimitGB)
	status.MinFreeSpaceBytes = gbToBytes(cfg.MinFreeSpaceGB)
	if cfg.RecordingDir != "" {
		if freeBytes, err := freeBytesForPath(cfg.RecordingDir); err == nil {
			status.FreeSpaceBytes = freeBytes
		}
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

func describeOBSConnectionError(err error) error {
	if err == nil {
		return nil
	}
	if classifyConnectionError(err) == "auth_failed" {
		return fmt.Errorf("OBS authentication failed: %w", err)
	}
	return fmt.Errorf("OBS connection failed or was lost: %w", err)
}

func (s *Service) activeRecordingForRun(runID string) (models.RecordingRecord, bool, error) {
	records, err := s.List()
	if err != nil {
		return models.RecordingRecord{}, false, err
	}
	for _, record := range records {
		if record.RunID != runID {
			continue
		}
		if record.Status == models.RecordingStatusFailed || record.Status == models.RecordingStatusMissing || record.Status == models.RecordingStatusSkipped {
			continue
		}
		return record, true, nil
	}
	return models.RecordingRecord{}, false, nil
}

func (s *Service) autoMetadataExistsForRun(runID string) (bool, error) {
	records, err := s.List()
	if err != nil {
		return false, err
	}
	for _, record := range records {
		if record.RunID != runID {
			continue
		}
		if record.Status == models.RecordingStatusFailed || record.Status == models.RecordingStatusMissing {
			continue
		}
		return true, nil
	}
	return false, nil
}

func (s *Service) List() ([]models.RecordingRecord, error) {
	if s == nil || s.metadata == nil {
		return []models.RecordingRecord{}, nil
	}
	return s.metadata.List()
}

func newRecordingRecord(rec models.RunRecord, reason models.RecordingKeepReason, pbAtSave bool) models.RecordingRecord {
	now := time.Now().UTC().Format(time.RFC3339)
	return models.RecordingRecord{
		ID:          newRecordingID(),
		RunID:       rec.RunID,
		RunFileName: rec.FileName,
		RunFilePath: rec.FilePath,
		Scenario:    statString(rec.Stats, "Scenario"),
		Score:       statFloat(rec.Stats, "Score"),
		PlayedAt:    statString(rec.Stats, "Date Played"),
		KeepReason:  reason,
		Status:      models.RecordingStatusPending,
		PBAtSave:    pbAtSave,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func newRecordingID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err == nil {
		return "rec_" + hex.EncodeToString(b[:])
	}
	return fmt.Sprintf("rec_%d", time.Now().UnixNano())
}

func statString(stats map[string]any, key string) string {
	if stats == nil {
		return ""
	}
	switch v := stats[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case nil:
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func statFloat(stats map[string]any, key string) float64 {
	if stats == nil {
		return 0
	}
	switch v := stats[key].(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case int32:
		return float64(v)
	default:
		return 0
	}
}

func (s *Service) RecordingDir() string {
	if s == nil || s.settingsSvc == nil {
		return ""
	}
	return s.settingsSvc.Get().Recording.RecordingDir
}
