package recording

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type videoToolchain struct {
	FFmpeg  string
	FFprobe string
	Source  string
}

var (
	findVideoToolchain = defaultFindVideoToolchain
	probeVideoDuration = defaultProbeVideoDuration
	trimReplayVideo    = defaultTrimReplayVideo
)

// defaultProbeVideoDuration reads container duration so absolute OBS timing can be mapped onto file offsets.
func defaultProbeVideoDuration(ctx context.Context, toolchain videoToolchain, path string) (time.Duration, error) {
	if strings.TrimSpace(path) == "" {
		return 0, errors.New("video path is empty")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, toolchain.FFprobe,
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	)
	configureVideoCommand(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe failed: %s", strings.TrimSpace(stderr.String()))
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil || seconds <= 0 {
		return 0, fmt.Errorf("ffprobe returned invalid duration %q", strings.TrimSpace(string(out)))
	}
	return time.Duration(seconds * float64(time.Second)), nil
}

// defaultTrimReplayVideo re-encodes the selected window and probes the result before the raw replay can be removed.
func defaultTrimReplayVideo(ctx context.Context, toolchain videoToolchain, sourcePath, targetPath string, plan replayClipPlan) (int64, time.Duration, error) {
	if plan.Offset < 0 || plan.Duration <= 0 {
		return 0, 0, errors.New("trim plan has invalid offsets")
	}

	ctx, cancel := context.WithTimeout(ctx, trimTimeout(plan.Duration))
	defer cancel()

	cmd := exec.CommandContext(ctx, toolchain.FFmpeg,
		"-y",
		"-hide_banner",
		"-loglevel", "error",
		"-ss", formatFFmpegSeconds(plan.Offset),
		"-i", sourcePath,
		"-t", formatFFmpegSeconds(plan.Duration),
		"-map", "0:v:0",
		"-map", "0:a?",
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-crf", "18",
		"-c:a", "aac",
		"-movflags", "+faststart",
		targetPath,
	)
	configureVideoCommand(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(targetPath)
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return 0, 0, fmt.Errorf("ffmpeg trim failed: %s", msg)
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		return 0, 0, fmt.Errorf("trimmed replay file is missing after ffmpeg completed: %w", err)
	}
	if info.IsDir() || info.Size() <= 0 {
		_ = os.Remove(targetPath)
		return 0, 0, errors.New("trimmed replay file is empty or invalid")
	}
	duration, err := probeVideoDuration(ctx, toolchain, targetPath)
	if err != nil {
		_ = os.Remove(targetPath)
		return 0, 0, fmt.Errorf("trimmed replay validation failed: %w", err)
	}
	if duration <= 0 {
		_ = os.Remove(targetPath)
		return 0, 0, errors.New("trimmed replay duration is invalid")
	}
	return info.Size(), duration, nil
}

func trimTimeout(duration time.Duration) time.Duration {
	timeout := 2*time.Minute + duration*2
	if timeout < 5*time.Minute {
		return 5 * time.Minute
	}
	return timeout
}

func formatFFmpegSeconds(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return strconv.FormatFloat(d.Seconds(), 'f', 3, 64)
}
