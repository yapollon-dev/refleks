package settings

import (
	"strings"

	"refleks/internal/models"
)

func recordingPasswordAfterLoad(r models.RecordingSettings) (models.RecordingSettings, bool) {
	if strings.TrimSpace(r.OBSPasswordProtected) != "" {
		if password, err := unprotectSecret(r.OBSPasswordProtected); err == nil {
			r.OBSPassword = password
		}
		r.OBSPasswordSet = true
		return r, false
	}

	if strings.TrimSpace(r.OBSPassword) == "" {
		r.OBSPasswordSet = false
		return r, false
	}

	// Legacy plaintext settings are kept in memory for OBS and re-saved encrypted.
	r.OBSPasswordSet = true
	return r, true
}

func mergeRecordingPassword(next, current models.RecordingSettings) models.RecordingSettings {
	if strings.TrimSpace(next.OBSPassword) != "" {
		next.OBSPasswordSet = true
		next.OBSPasswordProtected = ""
		return next
	}
	if next.OBSPasswordSet && (strings.TrimSpace(current.OBSPassword) != "" || strings.TrimSpace(current.OBSPasswordProtected) != "") {
		next.OBSPassword = current.OBSPassword
		next.OBSPasswordProtected = current.OBSPasswordProtected
		next.OBSPasswordSet = true
		return next
	}
	next.OBSPassword = ""
	next.OBSPasswordProtected = ""
	next.OBSPasswordSet = false
	return next
}

func settingsForFrontend(s models.Settings) models.Settings {
	if strings.TrimSpace(s.Recording.OBSPassword) != "" || strings.TrimSpace(s.Recording.OBSPasswordProtected) != "" || s.Recording.OBSPasswordSet {
		s.Recording.OBSPasswordSet = true
	}
	s.Recording.OBSPassword = ""
	s.Recording.OBSPasswordProtected = ""
	return s
}

func settingsForDisk(s models.Settings) (models.Settings, error) {
	password := s.Recording.OBSPassword
	if strings.TrimSpace(password) == "" {
		s.Recording.OBSPassword = ""
		if !s.Recording.OBSPasswordSet {
			s.Recording.OBSPasswordProtected = ""
		}
		return s, nil
	}

	protected, err := protectSecret(password)
	if err != nil {
		return models.Settings{}, err
	}
	s.Recording.OBSPassword = ""
	s.Recording.OBSPasswordProtected = protected
	s.Recording.OBSPasswordSet = protected != ""
	return s, nil
}
