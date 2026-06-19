package settings

import (
	"path/filepath"
	"testing"

	"refleks/internal/constants"
	"refleks/internal/models"
)

func TestDefaultRecordingSettings(t *testing.T) {
	got := Default().Recording

	if got.Enabled {
		t.Fatalf("recording should be disabled by default")
	}
	if got.OBSHost != constants.DefaultOBSHost {
		t.Fatalf("OBS host = %q, want %q", got.OBSHost, constants.DefaultOBSHost)
	}
	if got.OBSPort != constants.DefaultOBSPort {
		t.Fatalf("OBS port = %d, want %d", got.OBSPort, constants.DefaultOBSPort)
	}
	if !got.AutoConnect || !got.AutoStartReplayBuffer {
		t.Fatalf("auto connect and auto-start replay buffer should default to true")
	}
	if got.SavePolicy != models.RecordingPolicyEveryRun {
		t.Fatalf("save policy = %q, want %q", got.SavePolicy, models.RecordingPolicyEveryRun)
	}
	wantSuffix := filepath.Join(constants.ConfigDirName, constants.RecordingsSubdirName)
	if filepath.Base(filepath.Dir(got.RecordingDir)) != constants.ConfigDirName || filepath.Base(got.RecordingDir) != constants.RecordingsSubdirName {
		t.Fatalf("recording dir = %q, want suffix %q", got.RecordingDir, wantSuffix)
	}
}

func TestSanitizeRecordingSettingsAppliesDefaults(t *testing.T) {
	got := Sanitize(models.Settings{}).Recording

	if got.OBSHost != constants.DefaultOBSHost {
		t.Fatalf("OBS host = %q, want default", got.OBSHost)
	}
	if got.OBSPort != constants.DefaultOBSPort {
		t.Fatalf("OBS port = %d, want default", got.OBSPort)
	}
	if got.SavePolicy != models.RecordingPolicyEveryRun {
		t.Fatalf("save policy = %q, want every_run", got.SavePolicy)
	}
	if got.RecordingDir == "" {
		t.Fatalf("recording dir should be defaulted")
	}
}

func TestSanitizeRecordingSettingsPreservesExplicitFalseBooleans(t *testing.T) {
	got := SanitizeRecordingSettings(models.RecordingSettings{
		OBSHost:               "localhost",
		OBSPort:               4456,
		AutoConnect:           false,
		AutoStartReplayBuffer: false,
		SavePolicy:            models.RecordingPolicyManualOnly,
		RecordingDir:          "C:/clips",
		StorageLimitGB:        10,
		MinFreeSpaceGB:        2,
	})

	if got.AutoConnect || got.AutoStartReplayBuffer {
		t.Fatalf("explicit false recording booleans should be preserved")
	}
	if got.RecordingDir != filepath.Clean("C:/clips") {
		t.Fatalf("recording dir = %q, want cleaned path", got.RecordingDir)
	}
}
