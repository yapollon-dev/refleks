//go:build !windows

package recording

func defaultDetectOBSInstallation() obsInstallInfo {
	return obsInstallInfo{Status: "unknown"}
}
