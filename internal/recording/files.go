package recording

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	fileStableTimeout  = 10 * time.Second
	fileStableInterval = 200 * time.Millisecond
)

func waitForStableFile(path string) (os.FileInfo, error) {
	deadline := time.Now().Add(fileStableTimeout)
	var lastSize int64 = -1
	var stableCount int

	for {
		info, err := os.Stat(path)
		if err != nil {
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("OBS saved replay file is missing: %w", err)
			}
			time.Sleep(fileStableInterval)
			continue
		}
		if info.IsDir() {
			return nil, errors.New("OBS saved replay path is a directory")
		}
		if info.Size() == lastSize {
			stableCount++
			if stableCount >= 2 {
				return info, nil
			}
		} else {
			lastSize = info.Size()
			stableCount = 0
		}
		if time.Now().After(deadline) {
			return nil, errors.New("OBS saved replay file did not become stable")
		}
		time.Sleep(fileStableInterval)
	}
}

func moveRecordingFile(sourcePath, recordingDir, baseName string) (string, int64, error) {
	return moveRecordingFileWithCheck(sourcePath, recordingDir, baseName, nil)
}

func moveRecordingFileWithCheck(sourcePath, recordingDir, baseName string, check func(size int64) error) (string, int64, error) {
	if strings.TrimSpace(sourcePath) == "" {
		return "", 0, errors.New("OBS did not return a saved replay path")
	}
	info, err := waitForStableFile(sourcePath)
	if err != nil {
		return "", 0, err
	}
	if check != nil {
		if err := check(info.Size()); err != nil {
			return "", 0, err
		}
	}
	if err := os.MkdirAll(recordingDir, 0o755); err != nil {
		return "", 0, fmt.Errorf("failed to create recording folder: %w", err)
	}

	targetPath := uniqueRecordingPath(recordingDir, baseName, filepath.Ext(sourcePath))
	if err := os.Rename(sourcePath, targetPath); err == nil {
		return targetPath, info.Size(), nil
	}

	if err := copyFile(sourcePath, targetPath); err != nil {
		return "", 0, fmt.Errorf("failed to move OBS replay into recording folder: %w", err)
	}
	_ = os.Remove(sourcePath)
	return targetPath, info.Size(), nil
}

func uniqueRecordingPath(recordingDir, baseName, ext string) string {
	name := sanitizeFileName(baseName)
	if name == "" {
		name = "recording"
	}
	if ext == "" {
		ext = ".mp4"
	}

	candidate := filepath.Join(recordingDir, name+ext)
	if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
		return candidate
	}
	for i := 2; ; i++ {
		candidate = filepath.Join(recordingDir, fmt.Sprintf("%s-%d%s", name, i, ext))
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
	}
}

func sanitizeFileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"<", "_", ">", "_", ":", "_", `"`, "_", "/", "_", `\`, "_", "|", "_", "?", "_", "*", "_",
	)
	return strings.Trim(replacer.Replace(name), " .")
}

func copyFile(sourcePath, targetPath string) error {
	src, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		_ = os.Remove(targetPath)
		return err
	}
	return dst.Close()
}
