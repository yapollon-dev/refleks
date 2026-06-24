//go:build !windows

package recording

import "os/exec"

func configureVideoCommand(cmd *exec.Cmd) {
}
