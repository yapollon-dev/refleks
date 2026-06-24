package recording

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

// configureVideoCommand prevents ffmpeg and ffprobe from flashing a console window in the GUI app.
func configureVideoCommand(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}
