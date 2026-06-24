package recording

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"refleks/internal/models"
)

func TestFFmpegCandidateFromConfiguredExecutable(t *testing.T) {
	candidate := ffmpegCandidateFromPath(ffmpegSourceCustom, filepath.Join("C:", "tools", "ffmpeg", "bin", "ffmpeg.exe"))
	if !strings.HasSuffix(strings.ToLower(candidate.ffmpeg), filepath.Join("ffmpeg", "bin", "ffmpeg.exe")) {
		t.Fatalf("ffmpeg path = %q", candidate.ffmpeg)
	}
	if filepath.Base(candidate.ffprobe) != "ffprobe.exe" {
		t.Fatalf("ffprobe sibling = %q, want ffprobe.exe", candidate.ffprobe)
	}
}

func TestInspectFFmpegToolchainUsesCustomPathBeforeFallbacks(t *testing.T) {
	resetFFmpegTestCache(t)
	dir := t.TempDir()
	ffmpeg := writeFakeExecutable(t, filepath.Join(dir, "ffmpeg.exe"))
	ffprobe := writeFakeExecutable(t, filepath.Join(dir, "ffprobe.exe"))

	previousValidate := validateFFmpegCandidateCached
	validateFFmpegCandidateCached = func(candidate ffmpegCandidate) ffmpegInspection {
		return ffmpegInspection{
			Status:         ffmpegStatusWorking,
			Source:         candidate.source,
			FFmpegPath:     candidate.ffmpeg,
			FFprobePath:    candidate.ffprobe,
			FFmpegVersion:  "ffmpeg test",
			FFprobeVersion: "ffprobe test",
			Capabilities:   "validated",
			Toolchain:      videoToolchain{FFmpeg: candidate.ffmpeg, FFprobe: candidate.ffprobe, Source: candidate.source},
		}
	}
	t.Cleanup(func() { validateFFmpegCandidateCached = previousValidate })

	inspection := defaultInspectFFmpegToolchain(models.RecordingSettings{FFmpegPath: ffmpeg})
	if inspection.Status != ffmpegStatusWorking || inspection.Source != ffmpegSourceCustom {
		t.Fatalf("inspection = %#v, want custom working", inspection)
	}
	if inspection.Toolchain.FFmpeg != ffmpeg || inspection.Toolchain.FFprobe != ffprobe {
		t.Fatalf("toolchain paths = %#v", inspection.Toolchain)
	}
}

func TestFFmpegCapabilityCacheRevalidatesWhenBinaryChanges(t *testing.T) {
	resetFFmpegTestCache(t)
	dir := t.TempDir()
	ffmpeg := writeFakeExecutable(t, filepath.Join(dir, "ffmpeg.exe"))
	ffprobe := writeFakeExecutable(t, filepath.Join(dir, "ffprobe.exe"))

	calls := 0
	previousValidate := validateFFmpegCandidateCached
	validateFFmpegCandidateCached = func(candidate ffmpegCandidate) ffmpegInspection {
		calls++
		return ffmpegInspection{
			Status:      ffmpegStatusWorking,
			Source:      candidate.source,
			FFmpegPath:  candidate.ffmpeg,
			FFprobePath: candidate.ffprobe,
			Toolchain:   videoToolchain{FFmpeg: candidate.ffmpeg, FFprobe: candidate.ffprobe, Source: candidate.source},
		}
	}
	t.Cleanup(func() { validateFFmpegCandidateCached = previousValidate })

	candidate := ffmpegCandidate{source: ffmpegSourceCustom, ffmpeg: ffmpeg, ffprobe: ffprobe}
	_ = inspectFFmpegCandidate(candidate)
	_ = inspectFFmpegCandidate(candidate)
	if calls != 1 {
		t.Fatalf("validation calls = %d, want cached single call", calls)
	}

	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(ffmpeg, []byte("changed"), 0o755); err != nil {
		t.Fatalf("change fake ffmpeg: %v", err)
	}
	_ = inspectFFmpegCandidate(candidate)
	if calls != 2 {
		t.Fatalf("validation calls after binary change = %d, want 2", calls)
	}
}

func TestRecordingStatusIncludesDependencyStates(t *testing.T) {
	previousInspect := inspectFFmpegToolchain
	previousOBS := detectOBSInstallation
	inspectFFmpegToolchain = func(cfg models.RecordingSettings) ffmpegInspection {
		return ffmpegInspection{
			Status:        ffmpegStatusWorking,
			Source:        ffmpegSourceBundled,
			FFmpegPath:    filepath.Join("C:", "App", "ffmpeg", "bin", "ffmpeg.exe"),
			Capabilities:  "validated",
			FFmpegVersion: "ffmpeg test",
		}
	}
	detectOBSInstallation = func() obsInstallInfo {
		return obsInstallInfo{Status: "missing"}
	}
	t.Cleanup(func() {
		inspectFFmpegToolchain = previousInspect
		detectOBSInstallation = previousOBS
	})

	service := newStorageTestService(t, models.RecordingSettings{
		OBSHost:      "127.0.0.1",
		OBSPort:      4455,
		RecordingDir: t.TempDir(),
	})
	status := service.Status()
	if status.FFmpegStatus != ffmpegStatusWorking || status.FFmpegSource != ffmpegSourceBundled {
		t.Fatalf("ffmpeg status = %#v", status)
	}
	if status.OBSInstallStatus != "missing" {
		t.Fatalf("obs install status = %q, want missing", status.OBSInstallStatus)
	}
	if !status.OBSConnectionDetailsSaved || status.OBSWebSocketVerified {
		t.Fatalf("obs connection details/verification mismatch: %#v", status)
	}
}

func resetFFmpegTestCache(t *testing.T) {
	t.Helper()
	ffmpegCapabilityCache.Lock()
	ffmpegCapabilityCache.loaded = true
	ffmpegCapabilityCache.entries = make(map[string]ffmpegInspection)
	ffmpegCapabilityCache.Unlock()
}

func writeFakeExecutable(t *testing.T, path string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte("fake"), 0o755); err != nil {
		t.Fatalf("write fake executable: %v", err)
	}
	return filepath.Clean(path)
}
