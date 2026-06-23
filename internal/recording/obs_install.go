package recording

type obsInstallInfo struct {
	Status  string
	Path    string
	Version string
}

var detectOBSInstallation = defaultDetectOBSInstallation
