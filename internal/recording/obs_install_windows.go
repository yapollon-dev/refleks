//go:build windows

package recording

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

func defaultDetectOBSInstallation() obsInstallInfo {
	if info, ok := detectOBSFromRegistry(); ok {
		return info
	}
	if info, ok := detectOBSFromCommonPaths(); ok {
		return info
	}
	return obsInstallInfo{Status: "missing"}
}

func detectOBSFromRegistry() (obsInstallInfo, bool) {
	roots := []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER}
	paths := []string{
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`,
		`SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`,
	}
	for _, root := range roots {
		for _, path := range paths {
			key, err := registry.OpenKey(root, path, registry.READ)
			if err != nil {
				continue
			}
			names, err := key.ReadSubKeyNames(-1)
			_ = key.Close()
			if err != nil {
				continue
			}
			for _, name := range names {
				info, ok := readOBSUninstallEntry(root, filepath.Join(path, name))
				if ok {
					return info, true
				}
			}
		}
	}
	return obsInstallInfo{}, false
}

func readOBSUninstallEntry(root registry.Key, path string) (obsInstallInfo, bool) {
	key, err := registry.OpenKey(root, path, registry.READ)
	if err != nil {
		return obsInstallInfo{}, false
	}
	defer key.Close()

	displayName, _, err := key.GetStringValue("DisplayName")
	if err != nil || !strings.Contains(strings.ToLower(displayName), "obs studio") {
		return obsInstallInfo{}, false
	}
	version, _, _ := key.GetStringValue("DisplayVersion")
	installLocation, _, _ := key.GetStringValue("InstallLocation")
	displayIcon, _, _ := key.GetStringValue("DisplayIcon")
	exePath := obsExecutableFromInstallLocation(installLocation)
	if exePath == "" && displayIcon != "" {
		exePath = strings.Trim(strings.Split(displayIcon, ",")[0], `"`)
	}
	return obsInstallInfo{Status: "installed", Path: exePath, Version: version}, true
}

func detectOBSFromCommonPaths() (obsInstallInfo, bool) {
	for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
		if strings.TrimSpace(base) == "" {
			continue
		}
		path := filepath.Join(base, "obs-studio", "bin", "64bit", "obs64.exe")
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return obsInstallInfo{Status: "installed", Path: path}, true
		}
	}
	return obsInstallInfo{}, false
}

func obsExecutableFromInstallLocation(installLocation string) string {
	installLocation = strings.TrimSpace(strings.Trim(installLocation, `"`))
	if installLocation == "" {
		return ""
	}
	candidates := []string{
		filepath.Join(installLocation, "bin", "64bit", "obs64.exe"),
		filepath.Join(installLocation, "obs64.exe"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return candidates[0]
}
