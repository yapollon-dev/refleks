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

var errRecordingAlreadyExists = errors.New("recording already exists for this run")

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
	if _, err := s.ensureReplayBuffer(ctx, client, cfg); err != nil {
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

func (s *Service) SaveCurrentReplay(ctx context.Context, runID string) (models.RecordingRecord, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return s.saveUnlinkedReplay(ctx)
	}

	rec, err := s.runByID(runID)
	if err != nil {
		return models.RecordingRecord{}, err
	}
	return s.saveRunReplay(ctx, rec, models.RecordingKeepReasonManual, false, time.Time{}, models.RecordingLinkSourceManualSelected, false)
}

func (s *Service) SaveLatestRunReplay(ctx context.Context) (models.RecordingRecord, error) {
	return s.SaveCurrentReplay(ctx, "")
}

func (s *Service) SaveRunReplay(ctx context.Context, rec models.RunRecord) (models.RecordingRecord, error) {
	return s.saveRunReplay(ctx, rec, models.RecordingKeepReasonManual, false, time.Time{}, models.RecordingLinkSourceManualSelected, false)
}

func (s *Service) HandleCompletedRun(ctx context.Context, rec models.RunRecord) (models.RecordingRecord, error) {
	if s == nil || s.settingsSvc == nil || s.runStore == nil {
		return models.RecordingRecord{}, errors.New("recording service is not initialized")
	}
	importedAt := time.Now().UTC()

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
		record, err := s.upsertAutoAttemptStatus(rec, decision.Reason, decision.PBAtSave, importedAt, models.RecordingStatusSkipped, "")
		if err != nil {
			return models.RecordingRecord{}, err
		}
		return record, nil
	}
	if !cfg.AutoConnect {
		record, err := s.upsertAutoAttemptStatus(rec, decision.Reason, decision.PBAtSave, importedAt, models.RecordingStatusFailed, "automatic OBS connection is disabled")
		if err != nil {
			return models.RecordingRecord{}, err
		}
		if record.Status != models.RecordingStatusFailed || strings.TrimSpace(record.LastError) == "" {
			return record, nil
		}
		return record, errors.New(record.LastError)
	}
	record, err := s.saveRunReplay(ctx, rec, decision.Reason, decision.PBAtSave, importedAt, models.RecordingLinkSourceAutoCompletedRun, true)
	if errors.Is(err, errRecordingAlreadyExists) {
		return record, nil
	}
	return record, err
}

func (s *Service) saveRunReplay(ctx context.Context, rec models.RunRecord, reason models.RecordingKeepReason, pbAtSave bool, importedAt time.Time, linkSource models.RecordingLinkSource, requireEnabled bool) (models.RecordingRecord, error) {
	if s == nil || s.settingsSvc == nil || s.metadata == nil {
		return models.RecordingRecord{}, errors.New("recording service is not initialized")
	}

	s.saveMu.Lock()
	defer s.saveMu.Unlock()

	cfg := s.settingsSvc.Get().Recording
	if requireEnabled && !cfg.Enabled {
		return models.RecordingRecord{}, errors.New("recording is disabled")
	}
	if strings.TrimSpace(cfg.RecordingDir) == "" {
		return models.RecordingRecord{}, errors.New("recording folder is not configured")
	}

	record, err := s.prepareRecordingAttemptLocked(rec, reason, pbAtSave, importedAt, linkSource)
	if err != nil {
		return record, err
	}
	if err := s.metadata.Upsert(record); err != nil {
		return models.RecordingRecord{}, err
	}

	rec = runs.EnsureRunID(rec)
	return s.savePreparedRecordingLocked(ctx, cfg, record, rec.FileName, importedAt)
}

func (s *Service) saveUnlinkedReplay(ctx context.Context) (models.RecordingRecord, error) {
	if s == nil || s.settingsSvc == nil || s.metadata == nil {
		return models.RecordingRecord{}, errors.New("recording service is not initialized")
	}

	s.saveMu.Lock()
	defer s.saveMu.Unlock()

	cfg := s.settingsSvc.Get().Recording
	if strings.TrimSpace(cfg.RecordingDir) == "" {
		return models.RecordingRecord{}, errors.New("recording folder is not configured")
	}

	record := newUnlinkedRecordingRecord()
	if err := s.metadata.Upsert(record); err != nil {
		return models.RecordingRecord{}, err
	}
	return s.savePreparedRecordingLocked(ctx, cfg, record, "manual-replay", time.Time{})
}

func (s *Service) savePreparedRecordingLocked(ctx context.Context, cfg models.RecordingSettings, record models.RecordingRecord, baseName string, importedAt time.Time) (models.RecordingRecord, error) {
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

	readiness, err := s.ensureReplayBuffer(obsCtx, client, cfg)
	record.ReplayBufferStatusAtSave = readiness.Status
	record.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	_ = s.metadata.Upsert(record)
	if err != nil {
		return fail(err)
	}
	if !readiness.ActiveBefore {
		return fail(errors.New("OBS replay buffer was not active before this save; it was started for future saves, but this replay cannot be safely saved"))
	}

	saveResult, err := client.SaveReplayBuffer(obsCtx)
	if err != nil {
		return fail(fmt.Errorf("OBS replay buffer save failed: %w", err))
	}
	record.OBSSourcePath = saveResult.Path
	record.CaptureRequestedAt = saveResult.RequestedAt.Format(time.RFC3339Nano)
	record.CaptureConfirmedAt = saveResult.ConfirmedAt.Format(time.RFC3339Nano)
	record.OBSReplayFileModTime = saveResult.FileModTime.Format(time.RFC3339Nano)
	record.CaptureDelayMs = captureDelayMilliseconds(importedAt, saveResult.RequestedAt)
	if err := s.metadata.Upsert(record); err != nil {
		return fail(err)
	}

	finalPath, size, err := moveRecordingFileWithCheck(saveResult.Path, cfg.RecordingDir, baseName, func(size int64) error {
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

func (s *Service) upsertAutoAttemptStatus(rec models.RunRecord, reason models.RecordingKeepReason, pbAtSave bool, importedAt time.Time, status models.RecordingStatus, lastError string) (models.RecordingRecord, error) {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()

	record, err := s.prepareRecordingAttemptLocked(rec, reason, pbAtSave, importedAt, models.RecordingLinkSourceAutoCompletedRun)
	if errors.Is(err, errRecordingAlreadyExists) {
		return record, nil
	}
	if err != nil {
		return models.RecordingRecord{}, err
	}

	record.Status = status
	record.LastError = lastError
	record.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := s.metadata.Upsert(record); err != nil {
		return models.RecordingRecord{}, err
	}
	return record, nil
}

func (s *Service) prepareRecordingAttemptLocked(rec models.RunRecord, reason models.RecordingKeepReason, pbAtSave bool, importedAt time.Time, linkSource models.RecordingLinkSource) (models.RecordingRecord, error) {
	rec = runs.EnsureRunID(rec)
	if existing, ok, err := s.recordingForRun(rec.RunID); err != nil {
		return models.RecordingRecord{}, err
	} else if ok {
		if existing.Status == models.RecordingStatusSaved || existing.Status == models.RecordingStatusPending {
			return existing, errRecordingAlreadyExists
		}
		return resetRecordingAttempt(existing, rec, reason, pbAtSave, importedAt, linkSource), nil
	}

	record := newRecordingRecord(rec, reason, pbAtSave)
	record.LinkSource = linkSource
	if !importedAt.IsZero() {
		record.RunImportedAt = importedAt.Format(time.RFC3339Nano)
	}
	return record, nil
}

// resetRecordingAttempt keeps the metadata identity for a retry while clearing stale save output.
func resetRecordingAttempt(existing models.RecordingRecord, rec models.RunRecord, reason models.RecordingKeepReason, pbAtSave bool, importedAt time.Time, linkSource models.RecordingLinkSource) models.RecordingRecord {
	record := newRecordingRecord(rec, reason, pbAtSave)
	record.ID = existing.ID
	record.CreatedAt = existing.CreatedAt
	if strings.TrimSpace(record.CreatedAt) == "" {
		record.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	record.Protected = existing.Protected
	record.LinkSource = linkSource
	if !importedAt.IsZero() {
		record.RunImportedAt = importedAt.Format(time.RFC3339Nano)
	}
	return record
}

type replayBufferReadiness struct {
	Status       string
	ActiveBefore bool
	Started      bool
}

func (s *Service) ensureReplayBuffer(ctx context.Context, client *OBSClient, cfg models.RecordingSettings) (replayBufferReadiness, error) {
	status, err := client.GetReplayBufferStatus(ctx)
	if err != nil {
		return replayBufferReadiness{Status: "unknown"}, fmt.Errorf("failed to read OBS replay buffer status: %w", err)
	}
	if status.Active {
		return replayBufferReadiness{Status: "active", ActiveBefore: true}, nil
	}
	if !cfg.AutoStartReplayBuffer {
		return replayBufferReadiness{Status: "inactive"}, errors.New("OBS replay buffer is inactive and auto-start is disabled")
	}
	if err := client.StartReplayBuffer(ctx); err != nil {
		return replayBufferReadiness{Status: "inactive"}, fmt.Errorf("failed to start OBS replay buffer: %w", err)
	}
	return replayBufferReadiness{Status: "started_after_save_request", Started: true}, nil
}

func captureDelayMilliseconds(importedAt, requestedAt time.Time) int64 {
	if importedAt.IsZero() || requestedAt.IsZero() {
		return 0
	}
	delay := requestedAt.Sub(importedAt)
	if delay < 0 {
		return 0
	}
	return delay.Milliseconds()
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

func (s *Service) recordingForRun(runID string) (models.RecordingRecord, bool, error) {
	records, err := s.List()
	if err != nil {
		return models.RecordingRecord{}, false, err
	}

	var retryable models.RecordingRecord
	foundRetryable := false
	for _, record := range records {
		if record.RunID != runID {
			continue
		}
		if record.Status == models.RecordingStatusSaved || record.Status == models.RecordingStatusPending {
			return record, true, nil
		}
		if !foundRetryable {
			retryable = record
			foundRetryable = true
		}
	}
	if foundRetryable {
		return retryable, true, nil
	}
	return models.RecordingRecord{}, false, nil
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

func newUnlinkedRecordingRecord() models.RecordingRecord {
	now := time.Now().UTC().Format(time.RFC3339)
	return models.RecordingRecord{
		ID:         newRecordingID(),
		Scenario:   "Unlinked replay",
		KeepReason: models.RecordingKeepReasonManual,
		LinkSource: models.RecordingLinkSourceUnlinked,
		Status:     models.RecordingStatusPending,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

func (s *Service) runByID(runID string) (models.RunRecord, error) {
	if s == nil || s.runStore == nil {
		return models.RunRecord{}, errors.New("run store is not initialized")
	}
	runsList, err := s.runStore.LoadAllRunSummaries()
	if err != nil {
		return models.RunRecord{}, err
	}
	for _, rec := range runsList {
		rec = runs.EnsureRunID(rec)
		if rec.RunID == runID {
			return rec, nil
		}
	}
	return models.RunRecord{}, fmt.Errorf("run %q was not found", runID)
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
