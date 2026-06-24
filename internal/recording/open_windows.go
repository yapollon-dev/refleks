//go:build windows

package recording

import (
	"os/exec"
	"path/filepath"
)

func openVideoPath(path string) error {
	return startCommand(openVideoCommand(path))
}

func revealVideoPath(path string) error {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return startCommand(revealVideoCommand(path))
}

func startCommand(spec commandSpec) error {
	cmd := exec.Command(spec.Name, spec.Args...)
	return cmd.Start()
}

func openVideoCommand(path string) commandSpec {
	return commandSpec{
		Name: "rundll32.exe",
		Args: []string{"url.dll,FileProtocolHandler", path},
	}
}

func revealVideoCommand(path string) commandSpec {
	return commandSpec{
		Name: "explorer.exe",
		Args: []string{"/select,", path},
	}
}
