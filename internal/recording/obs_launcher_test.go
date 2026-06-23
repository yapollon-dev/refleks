package recording

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"refleks/internal/models"
)

func fakeOBSExecutable(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "obs64.exe")
	if err := os.WriteFile(path, []byte("fake obs"), 0o644); err != nil {
		t.Fatalf("write fake OBS executable: %v", err)
	}
	return path
}

func withLauncherFakes(t *testing.T, running func(string) bool, start func(context.Context, string, obsLaunchOptions) error) {
	t.Helper()
	previousRunning := isOBSProcessRunning
	previousStart := startOBSProcess
	isOBSProcessRunning = running
	startOBSProcess = start
	t.Cleanup(func() {
		isOBSProcessRunning = previousRunning
		startOBSProcess = previousStart
	})
}

func withOBSDetection(t *testing.T, detect func() obsInstallInfo) {
	t.Helper()
	previous := detectOBSInstallation
	detectOBSInstallation = detect
	t.Cleanup(func() {
		detectOBSInstallation = previous
	})
}

func withShortOBSLaunchWait(t *testing.T) {
	t.Helper()
	withOBSLaunchRetryTimings(t, 10*time.Millisecond, time.Millisecond)
}

func withOBSLaunchRetryTimings(t *testing.T, wait time.Duration, interval time.Duration) {
	t.Helper()
	previousWait := obsWebSocketLaunchWait
	previousInterval := obsWebSocketRetryInterval
	obsWebSocketLaunchWait = wait
	obsWebSocketRetryInterval = interval
	t.Cleanup(func() {
		obsWebSocketLaunchWait = previousWait
		obsWebSocketRetryInterval = previousInterval
	})
}

func TestResolveOBSExecutableUsesCustomPath(t *testing.T) {
	exe := fakeOBSExecutable(t)

	resolution, err := resolveOBSExecutable(models.RecordingSettings{OBSExecutablePath: exe})
	if err != nil {
		t.Fatalf("resolve custom OBS executable: %v", err)
	}
	if resolution.Path != exe || resolution.ProcessName != "obs64.exe" || resolution.Source != "custom" {
		t.Fatalf("unexpected resolution: %#v", resolution)
	}
}

func TestResolveOBSExecutableRejectsInvalidCustomPathWithoutFallback(t *testing.T) {
	detected := fakeOBSExecutable(t)
	missing := filepath.Join(t.TempDir(), "missing-obs64.exe")
	withOBSDetection(t, func() obsInstallInfo {
		return obsInstallInfo{Status: "installed", Path: detected}
	})

	resolution, err := resolveOBSExecutable(models.RecordingSettings{OBSExecutablePath: missing})
	if err == nil {
		t.Fatalf("invalid custom OBS path should fail")
	}
	if resolution.Path != missing {
		t.Fatalf("resolution path = %q, want custom path %q", resolution.Path, missing)
	}
	if !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("error should explain invalid custom path, got %v", err)
	}
}

func TestLaunchOBSUsesCustomPathAndMinimizeOption(t *testing.T) {
	fake := newFakeOBSServer(t, nil)
	exe := fakeOBSExecutable(t)
	cfg := fake.settings(t, "")
	cfg.Enabled = true
	cfg.AutoConnect = true
	cfg.AutoStartReplayBuffer = false
	cfg.AutoLaunchOBS = true
	cfg.LaunchOBSMinimized = true
	cfg.OBSExecutablePath = exe
	cfg.RecordingDir = t.TempDir()
	service := newStorageTestService(t, cfg)

	started := 0
	var gotPath string
	var gotOptions obsLaunchOptions
	withLauncherFakes(t,
		func(string) bool { return false },
		func(_ context.Context, path string, options obsLaunchOptions) error {
			started++
			gotPath = path
			gotOptions = options
			return nil
		},
	)

	status := service.LaunchOBS(context.Background())
	if started != 1 {
		t.Fatalf("start calls = %d, want 1", started)
	}
	if gotPath != exe || !gotOptions.MinimizedToTray {
		t.Fatalf("launch args = %q %#v, want custom path with minimized option", gotPath, gotOptions)
	}
	if status.OBSLaunchStatus != obsLaunchStatusWebSocketConnected || status.LastConnectionStatus != "connected" {
		t.Fatalf("unexpected launch status: %#v", status)
	}
	if !status.OBSLaunchedByRefleks {
		t.Fatalf("status should report OBS launched by RefleK's")
	}
}

func TestLaunchOBSDoesNotStartSecondInstanceWhenAlreadyRunning(t *testing.T) {
	fake := newFakeOBSServer(t, nil)
	exe := fakeOBSExecutable(t)
	cfg := fake.settings(t, "")
	cfg.AutoStartReplayBuffer = false
	cfg.OBSExecutablePath = exe
	cfg.RecordingDir = t.TempDir()
	service := newStorageTestService(t, cfg)

	started := 0
	withLauncherFakes(t,
		func(string) bool { return true },
		func(context.Context, string, obsLaunchOptions) error {
			started++
			return nil
		},
	)

	status := service.LaunchOBS(context.Background())
	if started != 0 {
		t.Fatalf("OBS was already running; start calls = %d, want 0", started)
	}
	if status.OBSProcessStatus != obsProcessStatusRunning || status.OBSLaunchStatus != obsLaunchStatusAlreadyRunning {
		t.Fatalf("unexpected existing OBS status: %#v", status)
	}
	if status.LastConnectionStatus != "connected" {
		t.Fatalf("existing OBS should still be checked through WebSocket: %#v", status)
	}
}

func TestPrepareOBSOnStartupIsGatedAndRunsOnce(t *testing.T) {
	exe := fakeOBSExecutable(t)
	skipped := newStorageTestService(t, models.RecordingSettings{
		Enabled:           false,
		AutoConnect:       true,
		AutoLaunchOBS:     true,
		OBSExecutablePath: exe,
		RecordingDir:      t.TempDir(),
	})
	withLauncherFakes(t,
		func(string) bool { return false },
		func(context.Context, string, obsLaunchOptions) error {
			t.Fatalf("startup launch should be skipped when recording is disabled")
			return nil
		},
	)
	if status := skipped.PrepareOBSOnStartup(context.Background()); status.OBSLaunchStatus != obsLaunchStatusSkipped {
		t.Fatalf("disabled startup status = %#v, want skipped", status)
	}

	fake := newFakeOBSServer(t, nil)
	cfg := fake.settings(t, "")
	cfg.Enabled = true
	cfg.AutoConnect = true
	cfg.AutoLaunchOBS = true
	cfg.AutoStartReplayBuffer = false
	cfg.OBSExecutablePath = exe
	cfg.RecordingDir = t.TempDir()
	service := newStorageTestService(t, cfg)

	started := 0
	withLauncherFakes(t,
		func(string) bool { return false },
		func(context.Context, string, obsLaunchOptions) error {
			started++
			return nil
		},
	)

	first := service.PrepareOBSOnStartup(context.Background())
	second := service.PrepareOBSOnStartup(context.Background())
	if started != 1 {
		t.Fatalf("startup launch calls = %d, want 1", started)
	}
	if first.LastConnectionStatus != "connected" || second.LastConnectionStatus != "connected" {
		t.Fatalf("startup statuses should reuse successful connection state: first=%#v second=%#v", first, second)
	}
}

func TestLaunchOBSStartsReplayBufferThroughWebSocketWhenEnabled(t *testing.T) {
	fake := newFakeOBSServer(t, func(f *fakeOBSServer) {
		f.replayActive = false
	})
	exe := fakeOBSExecutable(t)
	cfg := fake.settings(t, "")
	cfg.AutoStartReplayBuffer = true
	cfg.OBSExecutablePath = exe
	cfg.RecordingDir = t.TempDir()
	service := newStorageTestService(t, cfg)

	withLauncherFakes(t,
		func(string) bool { return false },
		func(context.Context, string, obsLaunchOptions) error { return nil },
	)

	status := service.LaunchOBS(context.Background())
	if fake.startReplayRequests != 1 {
		t.Fatalf("StartReplayBuffer requests = %d, want 1", fake.startReplayRequests)
	}
	if status.LastReplayBufferStatus != "active" {
		t.Fatalf("replay buffer status = %#v, want active", status)
	}
}

func TestLaunchOBSRetriesUntilOBSReady(t *testing.T) {
	withOBSLaunchRetryTimings(t, time.Second, time.Millisecond)
	fake := newFakeOBSServer(t, func(f *fakeOBSServer) {
		f.replayStatusNotReadyCount = 2
	})
	exe := fakeOBSExecutable(t)
	cfg := fake.settings(t, "")
	cfg.AutoStartReplayBuffer = false
	cfg.OBSExecutablePath = exe
	cfg.RecordingDir = t.TempDir()
	service := newStorageTestService(t, cfg)

	withLauncherFakes(t,
		func(string) bool { return false },
		func(context.Context, string, obsLaunchOptions) error { return nil },
	)

	status := service.LaunchOBS(context.Background())
	if status.LastError != "" || status.OBSLaunchLastError != "" {
		t.Fatalf("OBS not-ready errors should be retried until status succeeds: %#v", status)
	}
	if status.LastConnectionStatus != "connected" || status.LastReplayBufferStatus != "active" {
		t.Fatalf("unexpected final OBS status after retry: %#v", status)
	}
}

func TestLaunchOBSReportsMissingExecutableWithoutBlocking(t *testing.T) {
	withShortOBSLaunchWait(t)
	withOBSDetection(t, func() obsInstallInfo {
		return obsInstallInfo{Status: "missing"}
	})
	withLauncherFakes(t,
		func(string) bool { return false },
		func(context.Context, string, obsLaunchOptions) error {
			t.Fatalf("missing OBS executable should not be launched")
			return nil
		},
	)
	service := newStorageTestService(t, models.RecordingSettings{RecordingDir: t.TempDir()})

	status := service.LaunchOBS(context.Background())
	if status.OBSLaunchStatus != obsLaunchStatusExecutableNotFound {
		t.Fatalf("launch status = %#v, want executable_not_found", status)
	}
	if status.OBSLaunchLastError == "" {
		t.Fatalf("missing executable should include actionable error")
	}
}
