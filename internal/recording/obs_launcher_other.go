//go:build !windows

package recording

import (
	"context"
	"errors"
)

func defaultIsOBSProcessRunning(processName string) bool {
	return false
}

func defaultStartOBSProcess(ctx context.Context, path string, options obsLaunchOptions) error {
	return errors.New("launching OBS is only supported on Windows")
}
