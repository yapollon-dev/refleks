package recording

import (
	"os/exec"
	"testing"
)

func TestConfigureVideoCommandHidesWindowsConsole(t *testing.T) {
	cmd := exec.Command("ffmpeg")
	configureVideoCommand(cmd)
	if cmd.SysProcAttr == nil {
		t.Fatalf("SysProcAttr should be configured")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatalf("HideWindow should be enabled")
	}
	if cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Fatalf("CREATE_NO_WINDOW flag should be enabled")
	}
}
