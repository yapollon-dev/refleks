package models

type RecordingPolicy string
type RecordingKeepReason string
type RecordingStatus string

const (
	RecordingPolicyEveryRun   RecordingPolicy = "every_run"
	RecordingPolicyNewPB      RecordingPolicy = "new_pb"
	RecordingPolicyPBTies     RecordingPolicy = "pb_and_ties"
	RecordingPolicyTopThree   RecordingPolicy = "top_three"
	RecordingPolicyManualOnly RecordingPolicy = "manual_only"

	RecordingKeepReasonEveryRun RecordingKeepReason = "every_run"
	RecordingKeepReasonManual   RecordingKeepReason = "manual_save"

	RecordingStatusPending RecordingStatus = "pending"
	RecordingStatusSaved   RecordingStatus = "saved"
	RecordingStatusFailed  RecordingStatus = "failed"
	RecordingStatusMissing RecordingStatus = "missing"
)

type RecordingSettings struct {
	Enabled               bool            `json:"enabled"`
	OBSHost               string          `json:"obsHost"`
	OBSPort               int             `json:"obsPort"`
	OBSPassword           string          `json:"obsPassword,omitempty"`
	AutoConnect           bool            `json:"autoConnect"`
	AutoStartReplayBuffer bool            `json:"autoStartReplayBuffer"`
	SavePolicy            RecordingPolicy `json:"savePolicy"`
	AlwaysSaveScenarios   []string        `json:"alwaysSaveScenarios,omitempty"`
	NeverSaveScenarios    []string        `json:"neverSaveScenarios,omitempty"`
	RecordingDir          string          `json:"recordingDir"`
	StorageLimitGB        int             `json:"storageLimitGb"`
	MinFreeSpaceGB        int             `json:"minFreeSpaceGb"`
	AutoCleanup           bool            `json:"autoCleanup"`
}

type RecordingRecord struct {
	ID            string              `json:"id"`
	RunID         string              `json:"runId"`
	RunFileName   string              `json:"runFileName"`
	RunFilePath   string              `json:"runFilePath"`
	Scenario      string              `json:"scenario"`
	Score         float64             `json:"score"`
	PlayedAt      string              `json:"playedAt,omitempty"`
	KeepReason    RecordingKeepReason `json:"keepReason"`
	OBSSourcePath string              `json:"obsSourcePath,omitempty"`
	VideoPath     string              `json:"videoPath"`
	SizeBytes     int64               `json:"sizeBytes"`
	Status        RecordingStatus     `json:"status"`
	Protected     bool                `json:"protected"`
	PBAtSave      bool                `json:"pbAtSave"`
	CreatedAt     string              `json:"createdAt"`
	UpdatedAt     string              `json:"updatedAt"`
	LastError     string              `json:"lastError,omitempty"`
}

type RecordingRuntimeStatus struct {
	Enabled             bool   `json:"enabled"`
	RecordingDir        string `json:"recordingDir"`
	MetadataPath        string `json:"metadataPath"`
	TotalRecordings     int    `json:"totalRecordings"`
	TotalSizeBytes      int64  `json:"totalSizeBytes"`
	ConnectionStatus    string `json:"connectionStatus"`
	ReplayBufferStatus  string `json:"replayBufferStatus"`
	OBSVersion          string `json:"obsVersion,omitempty"`
	OBSWebSocketVersion string `json:"obsWebSocketVersion,omitempty"`
	LastError           string `json:"lastError,omitempty"`
}
