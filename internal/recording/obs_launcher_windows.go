//go:build windows

package recording

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func defaultIsOBSProcessRunning(processName string) bool {
	processName = strings.TrimSpace(processName)
	if processName == "" {
		processName = "obs64.exe"
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snapshot)

	var pe32 windows.ProcessEntry32
	pe32.Size = uint32(unsafe.Sizeof(pe32))
	if err := windows.Process32First(snapshot, &pe32); err != nil {
		return false
	}

	for {
		name := windows.UTF16ToString(pe32.ExeFile[:])
		if strings.EqualFold(name, processName) {
			return true
		}
		if err := windows.Process32Next(snapshot, &pe32); err != nil {
			break
		}
	}
	return false
}

func defaultStartOBSProcess(ctx context.Context, path string, options obsLaunchOptions) error {
	args := []string{}
	if options.MinimizedToTray {
		args = append(args, "--minimize-to-tray")
	}
	cmd := exec.Command(path, args...)
	cmd.Dir = filepath.Dir(path)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return nil
}
