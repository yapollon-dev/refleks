package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"refleks/internal/constants"
	"refleks/internal/models"
)

// Service manages application settings.
type Service struct {
	mu      sync.RWMutex
	current models.Settings
}

// NewService creates a new settings service with default values.
func NewService() *Service {
	return &Service{
		current: Default(),
	}
}

// Load reads settings from disk. If the file doesn't exist, it returns defaults.
func (s *Service) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir, err := EnsureConfigDir()
	if err != nil {
		return err
	}

	path := filepath.Join(dir, constants.SettingsFileName)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		s.current = Default()
		return s.saveLocked()
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	loaded := Default()
	if err := json.NewDecoder(f).Decode(&loaded); err != nil {
		return err
	}

	sanitized := Sanitize(loaded)
	var changed bool
	sanitized.Recording, changed = recordingPasswordAfterLoad(sanitized.Recording)
	s.current = sanitized
	if changed {
		return s.saveLocked()
	}
	return nil
}

// Get returns a copy of the current settings.
func (s *Service) Get() models.Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

// GetForFrontend returns settings without exposing the decrypted OBS password.
func (s *Service) GetForFrontend() models.Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return settingsForFrontend(s.current)
}

// Update updates the settings and persists them to disk.
func (s *Service) Update(newSettings models.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	newSettings.Recording = mergeRecordingPassword(newSettings.Recording, s.current.Recording)
	s.current = Sanitize(newSettings)
	return s.saveLocked()
}

// saveLocked writes the current settings to disk. Caller must hold the lock.
func (s *Service) saveLocked() error {
	dir, err := EnsureConfigDir()
	if err != nil {
		return err
	}

	persisted, err := settingsForDisk(s.current)
	if err != nil {
		return err
	}

	path := filepath.Join(dir, constants.SettingsFileName)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(persisted)
}

// Helper to get favorite benchmarks (logic moved from app.go/settings.go)
func (s *Service) GetFavoriteBenchmarks() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current.FavoriteBenchmarks
}

// Helper to set favorite benchmarks
func (s *Service) SetFavoriteBenchmarks(ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current.FavoriteBenchmarks = ids
	return s.saveLocked()
}
