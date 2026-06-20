package settings

import (
	"os"
	"path/filepath"
	"strings"
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
	if got.PreRollSeconds != constants.DefaultRecordingPreRollSeconds || got.PostRollSeconds != constants.DefaultRecordingPostRollSeconds {
		t.Fatalf("pre/post roll = %d/%d, want %d/%d", got.PreRollSeconds, got.PostRollSeconds, constants.DefaultRecordingPreRollSeconds, constants.DefaultRecordingPostRollSeconds)
	}
	if got.KeepRawReplay {
		t.Fatalf("raw replay retention should default to disabled")
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
	if got.PreRollSeconds != constants.DefaultRecordingPreRollSeconds || got.PostRollSeconds != constants.DefaultRecordingPostRollSeconds {
		t.Fatalf("pre/post roll should be defaulted, got %d/%d", got.PreRollSeconds, got.PostRollSeconds)
	}
}

func TestSanitizeRecordingSettingsPreservesExplicitFalseBooleans(t *testing.T) {
	got := SanitizeRecordingSettings(models.RecordingSettings{
		OBSHost:               "localhost",
		OBSPort:               4456,
		AutoConnect:           false,
		AutoStartReplayBuffer: false,
		SavePolicy:            models.RecordingPolicyNewPB,
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

func TestSanitizeRecordingSettingsMigratesLegacyManualOnlyPolicy(t *testing.T) {
	got := SanitizeRecordingSettings(models.RecordingSettings{
		Enabled:      true,
		OBSHost:      "localhost",
		OBSPort:      4455,
		SavePolicy:   models.RecordingPolicy("manual_only"),
		RecordingDir: "C:/clips",
	})

	if got.Enabled {
		t.Fatalf("legacy manual_only should disable auto recording")
	}
	if got.SavePolicy != models.RecordingPolicyEveryRun {
		t.Fatalf("save policy = %q, want every_run", got.SavePolicy)
	}
}

func TestSanitizeRecordingSettingsDefaultsUnknownPolicy(t *testing.T) {
	got := SanitizeRecordingSettings(models.RecordingSettings{
		OBSHost:      "localhost",
		OBSPort:      4455,
		SavePolicy:   models.RecordingPolicy("custom"),
		RecordingDir: "C:/clips",
	})

	if got.SavePolicy != models.RecordingPolicyEveryRun {
		t.Fatalf("save policy = %q, want every_run", got.SavePolicy)
	}
}

func TestSettingsServiceProtectsRecordingPasswordOnDisk(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	service := NewService()
	settings := service.Get()
	settings.Recording.OBSPassword = "super-secret"
	settings.Recording.OBSPasswordSet = true
	if err := service.Update(settings); err != nil {
		t.Fatalf("update settings with password: %v", err)
	}

	path, err := Path()
	if err != nil {
		t.Fatalf("settings path: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	if strings.Contains(string(data), "super-secret") {
		t.Fatalf("settings file should not contain plaintext OBS password: %s", data)
	}
	if !strings.Contains(string(data), "obsPasswordProtected") {
		t.Fatalf("settings file should contain protected OBS password: %s", data)
	}

	frontend := service.GetForFrontend()
	if frontend.Recording.OBSPassword != "" || !frontend.Recording.OBSPasswordSet {
		t.Fatalf("frontend settings should hide password but preserve set flag: %#v", frontend.Recording)
	}

	reloaded := NewService()
	if err := reloaded.Load(); err != nil {
		t.Fatalf("reload settings: %v", err)
	}
	if got := reloaded.Get().Recording.OBSPassword; got != "super-secret" {
		t.Fatalf("reloaded password = %q, want original", got)
	}
}
