package settings

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"refleks/internal/constants"
	"refleks/internal/models"
)

// DefaultKovaaksInstallDir returns an OS-appropriate default Kovaak's install directory.
func DefaultKovaaksInstallDir() string {
	if env := strings.TrimSpace(GetEnv(constants.EnvKovaaksInstallDirVar)); env != "" {
		return NormalizeKovaaksInstallDir(env)
	}
	if runtime.GOOS == "windows" {
		return constants.DefaultWindowsKovaaksInstallDir
	}
	return ""
}

// NormalizeKovaaksInstallDir trims, normalizes separators, and cleans the install path.
func NormalizeKovaaksInstallDir(p string) string {
	p = strings.TrimSpace(ExpandPathPlaceholders(p))
	if p == "" {
		return ""
	}
	return filepath.Clean(p)
}

// ResolveKovaaksStatsDir derives the stats directory from the configured install directory.
func ResolveKovaaksStatsDir(installDir string) string {
	installDir = NormalizeKovaaksInstallDir(installDir)
	if installDir == "" {
		return ""
	}
	return filepath.Join(installDir, constants.KovaaksDataDirName, constants.KovaaksStatsDirName)
}

// DefaultRecordingDir returns the local folder used for OBS replay files.
func DefaultRecordingDir() string {
	base, err := GetConfigDir()
	if err != nil {
		return filepath.Join(constants.ConfigDirName, constants.RecordingsSubdirName)
	}
	return filepath.Join(base, constants.RecordingsSubdirName)
}

// DefaultRecordingSettings returns recording defaults without enabling recording.
func DefaultRecordingSettings() models.RecordingSettings {
	return models.RecordingSettings{
		Enabled:               false,
		OBSHost:               constants.DefaultOBSHost,
		OBSPort:               constants.DefaultOBSPort,
		AutoConnect:           true,
		AutoStartReplayBuffer: true,
		SavePolicy:            models.RecordingPolicyEveryRun,
		RecordingDir:          DefaultRecordingDir(),
		StorageLimitGB:        constants.DefaultRecordingStorageLimitGB,
		MinFreeSpaceGB:        constants.DefaultRecordingMinFreeSpaceGB,
		AutoCleanup:           false,
	}
}

// Default returns sane default settings for a fresh install.
func Default() models.Settings {
	return models.Settings{
		SteamInstallDir:      constants.DefaultWindowsSteamInstallDir,
		KovaaksInstallDir:    DefaultKovaaksInstallDir(),
		SessionGapMinutes:    constants.DefaultSessionGapMinutes,
		RecentRunsDays:       constants.DefaultRecentRunsDays,
		RecentRunsMinCount:   constants.DefaultRecentRunsMinCount,
		Theme:                constants.DefaultTheme,
		Font:                 constants.DefaultFont,
		MouseTrackingEnabled: true,
		MouseBufferMinutes:   constants.DefaultMouseBufferMinutes,
		AutostartEnabled:     false,
		AnonymousEnabled:     false,
		RunSyncEnabled:       true,
		Recording:            DefaultRecordingSettings(),
		LastSeenVersion:      "",
	}
}

// Sanitize applies defaults to zero/empty fields and returns the updated copy.
func Sanitize(s models.Settings) models.Settings {
	if strings.TrimSpace(s.SteamInstallDir) == "" {
		s.SteamInstallDir = constants.DefaultWindowsSteamInstallDir
	}
	s.KovaaksInstallDir = NormalizeKovaaksInstallDir(s.KovaaksInstallDir)
	if s.KovaaksInstallDir == "" {
		s.KovaaksInstallDir = DefaultKovaaksInstallDir()
	}
	if s.SessionGapMinutes <= 0 {
		s.SessionGapMinutes = constants.DefaultSessionGapMinutes
	}
	if s.RecentRunsDays <= 0 {
		s.RecentRunsDays = constants.DefaultRecentRunsDays
	}
	if s.RecentRunsMinCount <= 0 {
		s.RecentRunsMinCount = constants.DefaultRecentRunsMinCount
	}
	if strings.TrimSpace(s.Theme) == "" {
		s.Theme = constants.DefaultTheme
	}
	if strings.TrimSpace(s.Font) == "" {
		s.Font = constants.DefaultFont
	}
	if s.MouseBufferMinutes <= 0 {
		s.MouseBufferMinutes = constants.DefaultMouseBufferMinutes
	}
	if s.ScenarioNotes == nil {
		s.ScenarioNotes = make(map[string]models.ScenarioNote)
	}
	if s.SessionNotes == nil {
		s.SessionNotes = make(map[string]models.SessionNote)
	}
	s.Recording = SanitizeRecordingSettings(s.Recording)
	return s
}

// SanitizeRecordingSettings applies defaults to missing recording fields while preserving explicit false booleans.
func SanitizeRecordingSettings(r models.RecordingSettings) models.RecordingSettings {
	defaults := DefaultRecordingSettings()
	if recordingSettingsAbsent(r) {
		return defaults
	}
	if strings.TrimSpace(r.OBSHost) == "" {
		r.OBSHost = defaults.OBSHost
	}
	if r.OBSPort <= 0 {
		r.OBSPort = defaults.OBSPort
	}
	switch {
	case strings.TrimSpace(string(r.SavePolicy)) == "":
		r.SavePolicy = defaults.SavePolicy
	case string(r.SavePolicy) == "manual_only":
		// Legacy manual-only meant "no automatic saves"; migrate it without enabling auto recording.
		r.Enabled = false
		r.SavePolicy = defaults.SavePolicy
	case !validRecordingPolicy(r.SavePolicy):
		r.SavePolicy = defaults.SavePolicy
	}
	if strings.TrimSpace(r.RecordingDir) == "" {
		r.RecordingDir = defaults.RecordingDir
	} else {
		r.RecordingDir = filepath.Clean(ExpandPathPlaceholders(r.RecordingDir))
	}
	if r.StorageLimitGB <= 0 {
		r.StorageLimitGB = defaults.StorageLimitGB
	}
	if r.MinFreeSpaceGB <= 0 {
		r.MinFreeSpaceGB = defaults.MinFreeSpaceGB
	}
	if strings.TrimSpace(r.OBSPassword) != "" || strings.TrimSpace(r.OBSPasswordProtected) != "" {
		r.OBSPasswordSet = true
	}
	return r
}

func validRecordingPolicy(policy models.RecordingPolicy) bool {
	switch policy {
	case models.RecordingPolicyEveryRun, models.RecordingPolicyNewPB, models.RecordingPolicyPBTies, models.RecordingPolicyTopThree:
		return true
	default:
		return false
	}
}

func recordingSettingsAbsent(r models.RecordingSettings) bool {
	return !r.Enabled &&
		strings.TrimSpace(r.OBSHost) == "" &&
		r.OBSPort == 0 &&
		strings.TrimSpace(r.OBSPassword) == "" &&
		strings.TrimSpace(r.OBSPasswordProtected) == "" &&
		!r.OBSPasswordSet &&
		!r.AutoConnect &&
		!r.AutoStartReplayBuffer &&
		strings.TrimSpace(string(r.SavePolicy)) == "" &&
		len(r.AlwaysSaveScenarios) == 0 &&
		len(r.NeverSaveScenarios) == 0 &&
		strings.TrimSpace(r.RecordingDir) == "" &&
		r.StorageLimitGB == 0 &&
		r.MinFreeSpaceGB == 0 &&
		!r.AutoCleanup
}

// GetConfigDir returns the application config directory under the user's home dir: $HOME/.refleks
// It does not ensure the directory exists.
func GetConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, constants.ConfigDirName), nil
}

// EnsureConfigDir returns the application config directory, creating it if necessary.
func EnsureConfigDir() (string, error) {
	base, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", err
	}
	return base, nil
}

// ExpandPathPlaceholders normalizes a path string for the current OS. No placeholders are supported.
func ExpandPathPlaceholders(p string) string {
	if p == "" {
		return p
	}
	// Convert any forward slashes to OS-native separators
	return filepath.FromSlash(p)
}

// Path returns the settings file path under the user home config directory ($HOME/.refleks).
func Path() (string, error) {
	base, err := GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "settings.json"), nil
}
