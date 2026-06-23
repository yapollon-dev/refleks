package recording

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"refleks/internal/models"
)

const (
	obsProcessStatusRunning    = "running"
	obsProcessStatusNotRunning = "not_running"
	obsProcessStatusUnknown    = "unknown"

	obsLaunchStatusNotChecked          = "not_checked"
	obsLaunchStatusSkipped             = "skipped"
	obsLaunchStatusAlreadyRunning      = "already_running"
	obsLaunchStatusLaunched            = "launched"
	obsLaunchStatusExecutableNotFound  = "executable_not_found"
	obsLaunchStatusLaunchFailed        = "launch_failed"
	obsLaunchStatusWaitingForWebSocket = "waiting_for_websocket"
	obsLaunchStatusWebSocketConnected  = "websocket_connected"
)

var (
	obsWebSocketLaunchWait    = 60 * time.Second
	obsWebSocketRetryInterval = 2 * time.Second

	isOBSProcessRunning = defaultIsOBSProcessRunning
	startOBSProcess     = defaultStartOBSProcess
)

type obsExecutableResolution struct {
	Path        string
	ProcessName string
	Source      string
}

type obsLaunchOptions struct {
	MinimizedToTray bool
}

func (s *Service) PrepareOBSOnStartup(ctx context.Context) models.RecordingRuntimeStatus {
	if s == nil {
		return models.RecordingRuntimeStatus{OBSLaunchStatus: obsLaunchStatusSkipped}
	}
	var status models.RecordingRuntimeStatus
	s.obsStartupOnce.Do(func() {
		status = s.launchOBS(ctx, true)
	})
	if status.OBSLaunchStatus == "" {
		status = s.Status()
	}
	return status
}

func (s *Service) LaunchOBS(ctx context.Context) models.RecordingRuntimeStatus {
	return s.launchOBS(ctx, false)
}

func (s *Service) launchOBS(ctx context.Context, startup bool) models.RecordingRuntimeStatus {
	status := s.localStatus()
	checkedAt := time.Now().UTC().Format(time.RFC3339)
	status.OBSLaunchCheckedAt = checkedAt

	if s == nil || s.settingsSvc == nil {
		status.OBSLaunchStatus = obsLaunchStatusLaunchFailed
		status.OBSLaunchLastError = "recording service is not initialized"
		s.cacheStatus(status)
		return status
	}

	cfg := s.settingsSvc.Get().Recording
	if startup && (!cfg.Enabled || !cfg.AutoConnect || !cfg.AutoLaunchOBS) {
		status.OBSLaunchStatus = obsLaunchStatusSkipped
		status.OBSLaunchLastError = ""
		s.cacheStatus(status)
		return status
	}

	resolution, err := resolveOBSExecutable(cfg)
	processName := "obs64.exe"
	if resolution.ProcessName != "" {
		processName = resolution.ProcessName
	}
	running := isOBSProcessRunning(processName)
	if !running && processName != "obs64.exe" {
		running = isOBSProcessRunning("obs64.exe")
	}
	if running {
		status.OBSProcessStatus = obsProcessStatusRunning
		status.OBSLaunchStatus = obsLaunchStatusAlreadyRunning
		status.OBSLaunchPath = resolution.Path
		status.OBSLaunchLastError = ""
		s.cacheStatus(status)
		return s.waitForOBSWebSocket(ctx, cfg, status, false)
	}

	status.OBSProcessStatus = obsProcessStatusNotRunning
	if err != nil {
		status.OBSLaunchStatus = obsLaunchStatusExecutableNotFound
		status.OBSLaunchPath = resolution.Path
		status.OBSLaunchLastError = err.Error()
		s.cacheStatus(status)
		return status
	}

	status.OBSLaunchPath = resolution.Path
	if err := startOBSProcess(ctx, resolution.Path, obsLaunchOptions{MinimizedToTray: cfg.LaunchOBSMinimized}); err != nil {
		status.OBSLaunchStatus = obsLaunchStatusLaunchFailed
		status.OBSLaunchLastError = err.Error()
		s.cacheStatus(status)
		return status
	}

	status.OBSProcessStatus = obsProcessStatusRunning
	status.OBSLaunchStatus = obsLaunchStatusLaunched
	status.OBSLaunchedByRefleks = true
	status.OBSLaunchLastError = ""
	s.cacheStatus(status)
	return s.waitForOBSWebSocket(ctx, cfg, status, true)
}

func (s *Service) waitForOBSWebSocket(ctx context.Context, cfg models.RecordingSettings, base models.RecordingRuntimeStatus, launchedByRefleks bool) models.RecordingRuntimeStatus {
	waitCtx, cancel := context.WithTimeout(ctx, obsWebSocketLaunchWait)
	defer cancel()

	connectedLaunchStatus := obsLaunchStatusWebSocketConnected
	if base.OBSLaunchStatus == obsLaunchStatusAlreadyRunning {
		connectedLaunchStatus = obsLaunchStatusAlreadyRunning
	}
	status := base
	status.OBSLaunchStatus = obsLaunchStatusWaitingForWebSocket
	status.OBSLaunchedByRefleks = launchedByRefleks
	status.OBSLaunchLastError = ""
	s.cacheStatus(status)

	for {
		attemptCtx, attemptCancel := context.WithTimeout(waitCtx, 8*time.Second)
		var next models.RecordingRuntimeStatus
		if cfg.AutoStartReplayBuffer {
			next = s.StartReplayBuffer(attemptCtx)
		} else {
			next = s.TestConnection(attemptCtx)
		}
		attemptCancel()

		next.OBSProcessStatus = obsProcessStatusRunning
		next.OBSLaunchStatus = connectedLaunchStatus
		next.OBSLaunchPath = base.OBSLaunchPath
		next.OBSLaunchCheckedAt = base.OBSLaunchCheckedAt
		next.OBSLaunchedByRefleks = launchedByRefleks
		if next.LastConnectionStatus == "connected" && next.LastError == "" {
			next.OBSLaunchLastError = next.LastError
			s.cacheStatus(next)
			return next
		}
		if next.LastConnectionStatus == "auth_failed" {
			next.OBSLaunchStatus = obsLaunchStatusWaitingForWebSocket
			next.OBSLaunchLastError = next.LastError
			s.cacheStatus(next)
			return next
		}
		if next.LastConnectionStatus == "connected" && !isOBSNotReadyError(next.LastError) {
			next.OBSLaunchLastError = next.LastError
			s.cacheStatus(next)
			return next
		}

		status = next
		status.OBSLaunchStatus = obsLaunchStatusWaitingForWebSocket
		status.OBSLaunchLastError = status.LastError
		s.cacheStatus(status)

		select {
		case <-waitCtx.Done():
			if isOBSNotReadyError(status.LastError) {
				status.OBSLaunchLastError = "OBS WebSocket connected, but OBS did not become ready before the launch wait timeout: " + status.LastError
			} else {
				status.OBSLaunchLastError = "OBS WebSocket was not reachable before the launch wait timeout"
			}
			s.cacheStatus(status)
			return status
		case <-time.After(obsWebSocketRetryInterval):
		}
	}
}

func isOBSNotReadyError(message string) bool {
	return strings.Contains(strings.ToLower(message), "obs is not ready to perform the request")
}

func resolveOBSExecutable(cfg models.RecordingSettings) (obsExecutableResolution, error) {
	if custom := strings.TrimSpace(cfg.OBSExecutablePath); custom != "" {
		return validateOBSExecutable(custom, "custom")
	}
	info := detectOBSInstallation()
	if strings.TrimSpace(info.Path) == "" {
		return obsExecutableResolution{ProcessName: "obs64.exe", Source: "detected"}, errors.New("OBS executable was not found; install OBS or configure the OBS executable path")
	}
	return validateOBSExecutable(info.Path, "detected")
}

func validateOBSExecutable(path, source string) (obsExecutableResolution, error) {
	path = filepath.Clean(strings.Trim(path, `"`))
	resolution := obsExecutableResolution{
		Path:        path,
		ProcessName: filepath.Base(path),
		Source:      source,
	}
	if strings.TrimSpace(path) == "" {
		return resolution, errors.New("OBS executable path is empty")
	}
	info, err := os.Stat(path)
	if err != nil {
		return resolution, fmt.Errorf("OBS executable is unavailable at %s: %w", path, err)
	}
	if info.IsDir() {
		return resolution, fmt.Errorf("OBS executable path points to a directory: %s", path)
	}
	return resolution, nil
}
