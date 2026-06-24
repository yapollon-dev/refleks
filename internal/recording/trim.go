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
	FFmpeg           string
	FFprobe          string
	Source           string
	TrimVideoEncoder string
	TrimEncoderLabel string
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

	encoder := trimEncoderConfigFor(toolchain.TrimVideoEncoder)
	cmd := exec.CommandContext(ctx, toolchain.FFmpeg,
		"-y",
		"-hide_banner",
		"-loglevel", "error",
		"-ss", formatFFmpegSeconds(plan.Offset),
		"-i", sourcePath,
		"-t", formatFFmpegSeconds(plan.Duration),
		"-map", "0:v:0",
		"-map", "0:a?",
	)
	cmd.Args = append(cmd.Args, encoder.Args...)
	cmd.Args = append(cmd.Args, trimSharedOutputArgs(targetPath)...)
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

type trimEncoderConfig struct {
	Name  string
	Label string
	Args  []string
}

// trimEncoderConfigs orders encoders by expected speed while keeping exact frame-boundary trimming.
func trimEncoderConfigs() []trimEncoderConfig {
	return []trimEncoderConfig{
		{
			Name:  "h264_nvenc",
			Label: "NVIDIA NVENC H.264",
			Args:  []string{"-c:v", "h264_nvenc", "-preset", "p1", "-cq", "20", "-b:v", "0"},
		},
		{
			Name:  "h264_qsv",
			Label: "Intel Quick Sync H.264",
			Args:  []string{"-c:v", "h264_qsv", "-preset", "veryfast", "-global_quality", "20"},
		},
		{
			Name:  "h264_amf",
			Label: "AMD AMF H.264",
			Args:  []string{"-c:v", "h264_amf", "-quality", "speed", "-qp_i", "20", "-qp_p", "20"},
		},
		defaultTrimEncoderConfig(),
	}
}

func defaultTrimEncoderConfig() trimEncoderConfig {
	return trimEncoderConfig{
		Name:  "libx264",
		Label: "CPU x264 H.264",
		Args:  []string{"-c:v", "libx264", "-preset", "ultrafast", "-crf", "20"},
	}
}

func trimEncoderConfigFor(name string) trimEncoderConfig {
	name = strings.TrimSpace(name)
	if name == "" {
		return defaultTrimEncoderConfig()
	}
	for _, candidate := range trimEncoderConfigs() {
		if candidate.Name == name {
			return candidate
		}
	}
	return defaultTrimEncoderConfig()
}

// trimSharedOutputArgs keeps generated clips browser-seekable while preserving exact re-encoded boundaries.
func trimSharedOutputArgs(targetPath string) []string {
	return []string{
		"-force_key_frames", "expr:gte(t,n_forced*2)",
		"-c:a", "aac",
		"-movflags", "+faststart",
		targetPath,
	}
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
