package models

type RecordingPolicy string
type RecordingKeepReason string
type RecordingStatus string
type RecordingLinkSource string

const (
	RecordingPolicyEveryRun RecordingPolicy = "every_run"
	RecordingPolicyNewPB    RecordingPolicy = "new_pb"
	RecordingPolicyPBTies   RecordingPolicy = "pb_and_ties"
	RecordingPolicyTopThree RecordingPolicy = "top_three"

	RecordingKeepReasonEveryRun RecordingKeepReason = "every_run"
	RecordingKeepReasonManual   RecordingKeepReason = "manual_save"
	RecordingKeepReasonNewPB    RecordingKeepReason = "new_pb"
	RecordingKeepReasonPBTie    RecordingKeepReason = "pb_tie"
	RecordingKeepReasonTopThree RecordingKeepReason = "top_three"
	RecordingKeepReasonAlways   RecordingKeepReason = "always_save_scenario"
	RecordingKeepReasonNever    RecordingKeepReason = "never_save_scenario"

	RecordingStatusPending RecordingStatus = "pending"
	RecordingStatusSaved   RecordingStatus = "saved"
	RecordingStatusFailed  RecordingStatus = "failed"
	RecordingStatusMissing RecordingStatus = "missing"
	RecordingStatusSkipped RecordingStatus = "skipped"

	RecordingLinkSourceAutoCompletedRun RecordingLinkSource = "auto_completed_run"
	RecordingLinkSourceManualSelected   RecordingLinkSource = "manual_user_selected"
	RecordingLinkSourceUnlinked         RecordingLinkSource = "unlinked"
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
	ID                       string              `json:"id"`
	RunID                    string              `json:"runId"`
	RunFileName              string              `json:"runFileName"`
	RunFilePath              string              `json:"runFilePath"`
	Scenario                 string              `json:"scenario"`
	Score                    float64             `json:"score"`
	PlayedAt                 string              `json:"playedAt,omitempty"`
	KeepReason               RecordingKeepReason `json:"keepReason"`
	LinkSource               RecordingLinkSource `json:"linkSource,omitempty"`
	OBSSourcePath            string              `json:"obsSourcePath,omitempty"`
	RunImportedAt            string              `json:"runImportedAt,omitempty"`
	CaptureRequestedAt       string              `json:"captureRequestedAt,omitempty"`
	CaptureConfirmedAt       string              `json:"captureConfirmedAt,omitempty"`
	CaptureDelayMs           int64               `json:"captureDelayMs,omitempty"`
	ReplayBufferStatusAtSave string              `json:"replayBufferStatusAtSave,omitempty"`
	OBSReplayFileModTime     string              `json:"obsReplayFileModTime,omitempty"`
	VideoPath                string              `json:"videoPath"`
	SizeBytes                int64               `json:"sizeBytes"`
	Status                   RecordingStatus     `json:"status"`
	Protected                bool                `json:"protected"`
	PBAtSave                 bool                `json:"pbAtSave"`
	CreatedAt                string              `json:"createdAt"`
	UpdatedAt                string              `json:"updatedAt"`
	LastError                string              `json:"lastError,omitempty"`
}

type RecordingRuntimeStatus struct {
	Enabled             bool   `json:"enabled"`
	RecordingDir        string `json:"recordingDir"`
	MetadataPath        string `json:"metadataPath"`
	TotalRecordings     int    `json:"totalRecordings"`
	TotalSizeBytes      int64  `json:"totalSizeBytes"`
	StorageLimitBytes   int64  `json:"storageLimitBytes"`
	MinFreeSpaceBytes   int64  `json:"minFreeSpaceBytes"`
	FreeSpaceBytes      int64  `json:"freeSpaceBytes"`
	ConnectionStatus    string `json:"connectionStatus"`
	ReplayBufferStatus  string `json:"replayBufferStatus"`
	OBSVersion          string `json:"obsVersion,omitempty"`
	OBSWebSocketVersion string `json:"obsWebSocketVersion,omitempty"`
	LastError           string `json:"lastError,omitempty"`
}

type RecordingCleanupItem struct {
	ID         string              `json:"id"`
	Scenario   string              `json:"scenario"`
	VideoPath  string              `json:"videoPath"`
	SizeBytes  int64               `json:"sizeBytes"`
	KeepReason RecordingKeepReason `json:"keepReason"`
	CreatedAt  string              `json:"createdAt"`
	Protected  bool                `json:"protected"`
	PBAtSave   bool                `json:"pbAtSave"`
}

type RecordingCleanupPreview struct {
	TotalSizeBytes           int64                  `json:"totalSizeBytes"`
	StorageLimitBytes        int64                  `json:"storageLimitBytes"`
	MinFreeSpaceBytes        int64                  `json:"minFreeSpaceBytes"`
	FreeSpaceBytes           int64                  `json:"freeSpaceBytes"`
	RequiredBytes            int64                  `json:"requiredBytes"`
	ReclaimableBytes         int64                  `json:"reclaimableBytes"`
	PlannedDeleteBytes       int64                  `json:"plannedDeleteBytes"`
	PlannedDeleteCount       int                    `json:"plannedDeleteCount"`
	EligibleCount            int                    `json:"eligibleCount"`
	ExcludedProtectedCount   int                    `json:"excludedProtectedCount"`
	ExcludedPBCount          int                    `json:"excludedPbCount"`
	ExcludedUnavailableCount int                    `json:"excludedUnavailableCount"`
	WillMeetLimits           bool                   `json:"willMeetLimits"`
	Reason                   string                 `json:"reason"`
	Items                    []RecordingCleanupItem `json:"items"`
	DeletedCount             int                    `json:"deletedCount"`
	DeletedBytes             int64                  `json:"deletedBytes"`
}
