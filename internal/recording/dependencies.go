package recording

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"refleks/internal/models"
	appsettings "refleks/internal/settings"
)

const (
	ffmpegStatusWorking = "working"
	ffmpegStatusMissing = "missing"
	ffmpegStatusBroken  = "broken"

	ffmpegSourceCustom  = "custom"
	ffmpegSourceBundled = "bundled"
	ffmpegSourcePath    = "path"

	ffmpegCapabilityCacheVersion = "trim-encoder-v2"
)

type ffmpegInspection struct {
	Status         string
	Source         string
	FFmpegPath     string
	FFprobePath    string
	FFmpegVersion  string
	FFprobeVersion string
	Capabilities   string
	Error          string
	Toolchain      videoToolchain
}

type ffmpegCandidate struct {
	source  string
	ffmpeg  string
	ffprobe string
}

type ffmpegCacheEntry struct {
	key        string
	inspection ffmpegInspection
}

type ffmpegPersistentCache struct {
	Entries map[string]ffmpegInspection `json:"entries"`
}

var (
	inspectFFmpegToolchain        = defaultInspectFFmpegToolchain
	validateFFmpegCandidateCached = validateFFmpegCandidate

	ffmpegCapabilityCache = struct {
		sync.Mutex
		loaded  bool
		entries map[string]ffmpegInspection
	}{entries: make(map[string]ffmpegInspection)}
)

func defaultFindVideoToolchain(cfg models.RecordingSettings) (videoToolchain, error) {
	inspection := inspectFFmpegToolchain(cfg)
	if inspection.Status != ffmpegStatusWorking {
		if strings.TrimSpace(inspection.Error) != "" {
			return videoToolchain{}, errors.New(inspection.Error)
		}
		return videoToolchain{}, errors.New("ffmpeg and ffprobe were not found")
	}
	return inspection.Toolchain, nil
}

func defaultInspectFFmpegToolchain(cfg models.RecordingSettings) ffmpegInspection {
	candidates := ffmpegCandidates(cfg)
	if len(candidates) == 0 {
		return ffmpegInspection{Status: ffmpegStatusMissing, Error: "ffmpeg and ffprobe were not found in the custom path, bundled app folder, or PATH"}
	}

	var firstFailure ffmpegInspection
	for _, candidate := range candidates {
		inspection := inspectFFmpegCandidate(candidate)
		if inspection.Status == ffmpegStatusWorking {
			return inspection
		}
		if firstFailure.Status == "" || candidate.source == ffmpegSourceCustom {
			firstFailure = inspection
		}
	}
	if firstFailure.Status == "" {
		return ffmpegInspection{Status: ffmpegStatusMissing, Error: "ffmpeg and ffprobe were not found"}
	}
	return firstFailure
}

func ffmpegCandidates(cfg models.RecordingSettings) []ffmpegCandidate {
	var candidates []ffmpegCandidate
	seen := map[string]bool{}
	add := func(candidate ffmpegCandidate) {
		if strings.TrimSpace(candidate.ffmpeg) == "" || strings.TrimSpace(candidate.ffprobe) == "" {
			return
		}
		key := strings.ToLower(filepath.Clean(candidate.ffmpeg) + "|" + filepath.Clean(candidate.ffprobe))
		if seen[key] {
			return
		}
		seen[key] = true
		candidate.ffmpeg = filepath.Clean(candidate.ffmpeg)
		candidate.ffprobe = filepath.Clean(candidate.ffprobe)
		candidates = append(candidates, candidate)
	}

	if custom := strings.TrimSpace(cfg.FFmpegPath); custom != "" {
		add(ffmpegCandidateFromPath(ffmpegSourceCustom, custom))
	}
	if exe, err := os.Executable(); err == nil {
		add(ffmpegCandidate{
			source:  ffmpegSourceBundled,
			ffmpeg:  filepath.Join(filepath.Dir(exe), "ffmpeg", "bin", "ffmpeg.exe"),
			ffprobe: filepath.Join(filepath.Dir(exe), "ffmpeg", "bin", "ffprobe.exe"),
		})
	}
	if ffmpeg, err := exec.LookPath("ffmpeg"); err == nil {
		if ffprobe, err := exec.LookPath("ffprobe"); err == nil {
			add(ffmpegCandidate{source: ffmpegSourcePath, ffmpeg: ffmpeg, ffprobe: ffprobe})
		}
	}
	return candidates
}

func ffmpegCandidateFromPath(source, configured string) ffmpegCandidate {
	configured = filepath.Clean(configured)
	name := strings.ToLower(filepath.Base(configured))
	if name == "ffmpeg.exe" || name == "ffmpeg" {
		return ffmpegCandidate{source: source, ffmpeg: configured, ffprobe: filepath.Join(filepath.Dir(configured), executableName("ffprobe"))}
	}
	if name == "ffprobe.exe" || name == "ffprobe" {
		return ffmpegCandidate{source: source, ffmpeg: filepath.Join(filepath.Dir(configured), executableName("ffmpeg")), ffprobe: configured}
	}
	return ffmpegCandidate{source: source, ffmpeg: filepath.Join(configured, executableName("ffmpeg")), ffprobe: filepath.Join(configured, executableName("ffprobe"))}
}

func executableName(base string) string {
	if strings.HasSuffix(strings.ToLower(base), ".exe") {
		return base
	}
	return base + ".exe"
}

func inspectFFmpegCandidate(candidate ffmpegCandidate) ffmpegInspection {
	candidate.ffmpeg = filepath.Clean(candidate.ffmpeg)
	candidate.ffprobe = filepath.Clean(candidate.ffprobe)

	key, err := ffmpegCandidateCacheKey(candidate)
	if err != nil {
		return ffmpegInspection{
			Status:      ffmpegStatusMissing,
			Source:      candidate.source,
			FFmpegPath:  candidate.ffmpeg,
			FFprobePath: candidate.ffprobe,
			Error:       err.Error(),
		}
	}

	if cached, ok := cachedFFmpegInspection(key); ok {
		return cached
	}

	inspection := validateFFmpegCandidateCached(candidate)
	storeCachedFFmpegInspection(key, inspection)
	return inspection
}

func cachedFFmpegInspection(key string) (ffmpegInspection, bool) {
	ffmpegCapabilityCache.Lock()
	defer ffmpegCapabilityCache.Unlock()
	loadPersistentFFmpegCacheLocked()
	cached, ok := ffmpegCapabilityCache.entries[key]
	return cached, ok
}

func storeCachedFFmpegInspection(key string, inspection ffmpegInspection) {
	ffmpegCapabilityCache.Lock()
	defer ffmpegCapabilityCache.Unlock()
	loadPersistentFFmpegCacheLocked()
	ffmpegCapabilityCache.entries[key] = inspection
	savePersistentFFmpegCacheLocked()
}

func loadPersistentFFmpegCacheLocked() {
	if ffmpegCapabilityCache.loaded {
		return
	}
	ffmpegCapabilityCache.loaded = true
	path, err := ffmpegCachePath()
	if err != nil {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var cache ffmpegPersistentCache
	if err := json.Unmarshal(data, &cache); err != nil || cache.Entries == nil {
		return
	}
	for key, inspection := range cache.Entries {
		ffmpegCapabilityCache.entries[key] = inspection
	}
}

func savePersistentFFmpegCacheLocked() {
	path, err := ffmpegCachePath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	data, err := json.MarshalIndent(ffmpegPersistentCache{Entries: ffmpegCapabilityCache.entries}, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o644)
}

func ffmpegCachePath() (string, error) {
	base, err := appsettings.GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "recording-dependencies.json"), nil
}

func ffmpegCandidateCacheKey(candidate ffmpegCandidate) (string, error) {
	ffmpegInfo, err := os.Stat(candidate.ffmpeg)
	if err != nil {
		return "", fmt.Errorf("%s ffmpeg is unavailable at %s: %w", candidate.source, candidate.ffmpeg, err)
	}
	ffprobeInfo, err := os.Stat(candidate.ffprobe)
	if err != nil {
		return "", fmt.Errorf("%s ffprobe is unavailable at %s: %w", candidate.source, candidate.ffprobe, err)
	}
	if ffmpegInfo.IsDir() || ffprobeInfo.IsDir() {
		return "", fmt.Errorf("%s ffmpeg path points to a directory", candidate.source)
	}
	return strings.Join([]string{
		ffmpegCapabilityCacheVersion,
		candidate.source,
		filepath.Clean(candidate.ffmpeg),
		fmt.Sprint(ffmpegInfo.Size()),
		ffmpegInfo.ModTime().UTC().Format(time.RFC3339Nano),
		filepath.Clean(candidate.ffprobe),
		fmt.Sprint(ffprobeInfo.Size()),
		ffprobeInfo.ModTime().UTC().Format(time.RFC3339Nano),
	}, "|"), nil
}

func validateFFmpegCandidate(candidate ffmpegCandidate) ffmpegInspection {
	toolchain := videoToolchain{FFmpeg: candidate.ffmpeg, FFprobe: candidate.ffprobe, Source: candidate.source}
	inspection := ffmpegInspection{
		Status:      ffmpegStatusBroken,
		Source:      candidate.source,
		FFmpegPath:  candidate.ffmpeg,
		FFprobePath: candidate.ffprobe,
		Toolchain:   toolchain,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	ffmpegVersion, err := videoToolVersion(ctx, candidate.ffmpeg)
	if err != nil {
		inspection.Error = "ffmpeg version check failed: " + err.Error()
		return inspection
	}
	ffprobeVersion, err := videoToolVersion(ctx, candidate.ffprobe)
	if err != nil {
		inspection.Error = "ffprobe version check failed: " + err.Error()
		return inspection
	}
	inspection.FFmpegVersion = ffmpegVersion
	inspection.FFprobeVersion = ffprobeVersion

	toolchain, err = validateFFmpegCapabilities(ctx, toolchain)
	if err != nil {
		inspection.Error = "ffmpeg capability check failed: " + err.Error()
		return inspection
	}
	inspection.Toolchain = toolchain
	inspection.Status = ffmpegStatusWorking
	inspection.Capabilities = strings.Join([]string{
		"mp4_input",
		"ffprobe_duration",
		"trim_encoder:" + toolchain.TrimVideoEncoder,
		"aac",
		"optional_audio_map",
		"faststart",
	}, ",")
	inspection.Error = ""
	return inspection
}

func videoToolVersion(ctx context.Context, path string) (string, error) {
	cmd := exec.CommandContext(ctx, path, "-version")
	configureVideoCommand(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", errors.New(msg)
	}
	firstLine := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	if firstLine == "" {
		return "", errors.New("empty version output")
	}
	return firstLine, nil
}

// validateFFmpegCapabilities runs a tiny end-to-end encode and trim so status reflects the current trimming pipeline.
func validateFFmpegCapabilities(ctx context.Context, toolchain videoToolchain) (videoToolchain, error) {
	dir, err := os.MkdirTemp("", "refleks-ffmpeg-check-*")
	if err != nil {
		return toolchain, err
	}
	defer os.RemoveAll(dir)

	source := filepath.Join(dir, "source.mp4")
	if err := runVideoCommand(ctx, toolchain.FFmpeg,
		"-y",
		"-hide_banner",
		"-loglevel", "error",
		"-f", "lavfi",
		"-i", "testsrc=size=16x16:rate=1:duration=0.3",
		"-f", "lavfi",
		"-i", "anullsrc=channel_layout=mono:sample_rate=44100",
		"-t", "0.3",
		"-map", "0:v:0",
		"-map", "1:a:0",
		"-c:v", "mpeg4",
		"-q:v", "5",
		"-c:a", "aac",
		"-movflags", "+faststart",
		source,
	); err != nil {
		return toolchain, fmt.Errorf("sample MP4 encode failed: %w", err)
	}
	if _, err := defaultProbeVideoDuration(ctx, toolchain, source); err != nil {
		return toolchain, fmt.Errorf("sample duration probe failed: %w", err)
	}
	toolchain, err = selectTrimEncoder(ctx, toolchain, source, dir)
	if err != nil {
		return toolchain, err
	}
	return toolchain, nil
}

// selectTrimEncoder tries each exact H.264 trim encoder against a tiny sample so unsupported hardware paths are skipped.
func selectTrimEncoder(ctx context.Context, toolchain videoToolchain, source string, dir string) (videoToolchain, error) {
	var failures []string
	for _, encoder := range trimEncoderConfigs() {
		trimmed := filepath.Join(dir, "trimmed-"+encoder.Name+".mp4")
		args := []string{
			"-y",
			"-hide_banner",
			"-loglevel", "error",
			"-ss", "0",
			"-i", source,
			"-t", "0.1",
			"-map", "0:v:0",
			"-map", "0:a?",
		}
		args = append(args, encoder.Args...)
		args = append(args, "-c:a", "aac", "-movflags", "+faststart", trimmed)
		if err := runVideoCommand(ctx, toolchain.FFmpeg, args...); err != nil {
			failures = append(failures, encoder.Name+": "+err.Error())
			_ = os.Remove(trimmed)
			continue
		}
		info, err := os.Stat(trimmed)
		if err != nil {
			failures = append(failures, encoder.Name+": sample trimmed MP4 is missing")
			continue
		}
		if info.Size() <= 0 {
			failures = append(failures, encoder.Name+": sample trimmed MP4 is empty")
			_ = os.Remove(trimmed)
			continue
		}
		if _, err := defaultProbeVideoDuration(ctx, toolchain, trimmed); err != nil {
			failures = append(failures, encoder.Name+": sample trimmed duration probe failed: "+err.Error())
			_ = os.Remove(trimmed)
			continue
		}
		toolchain.TrimVideoEncoder = encoder.Name
		toolchain.TrimEncoderLabel = encoder.Label
		return toolchain, nil
	}
	if len(failures) == 0 {
		return toolchain, errors.New("no trim encoders were tested")
	}
	return toolchain, errors.New("no usable H.264 trim encoder found: " + strings.Join(failures, "; "))
}

func runVideoCommand(ctx context.Context, executable string, args ...string) error {
	cmd := exec.CommandContext(ctx, executable, args...)
	configureVideoCommand(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return errors.New(msg)
	}
	return nil
}

func applyFFmpegStatus(status *models.RecordingRuntimeStatus, cfg models.RecordingSettings) {
	if status == nil {
		return
	}
	inspection := inspectFFmpegToolchain(cfg)
	status.FFmpegStatus = inspection.Status
	status.FFmpegSource = inspection.Source
	status.FFmpegPath = inspection.FFmpegPath
	status.FFmpegVersion = inspection.FFmpegVersion
	status.FFprobeVersion = inspection.FFprobeVersion
	status.FFmpegCapabilities = inspection.Capabilities
	status.FFmpegError = inspection.Error
}

func applyOBSInstallStatus(status *models.RecordingRuntimeStatus, cfg models.RecordingSettings) {
	if status == nil {
		return
	}
	info := detectOBSInstallation()
	status.OBSInstallStatus = info.Status
	status.OBSInstallPath = info.Path
	status.OBSInstallVersion = info.Version
	status.OBSConnectionDetailsSaved = strings.TrimSpace(cfg.OBSHost) != "" && cfg.OBSPort > 0
	processName := "obs64.exe"
	if strings.TrimSpace(cfg.OBSExecutablePath) != "" {
		processName = filepath.Base(cfg.OBSExecutablePath)
	}
	if isOBSProcessRunning(processName) || (processName != "obs64.exe" && isOBSProcessRunning("obs64.exe")) {
		status.OBSProcessStatus = obsProcessStatusRunning
	} else {
		status.OBSProcessStatus = obsProcessStatusNotRunning
	}
	if status.OBSLaunchStatus == "" {
		status.OBSLaunchStatus = obsLaunchStatusNotChecked
	}
}
