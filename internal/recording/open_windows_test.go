//go:build windows

package recording

import "testing"

func TestWindowsOpenRevealCommands(t *testing.T) {
	path := `C:\recordings\clip one.mp4`

	open := openVideoCommand(path)
	if open.Name != "rundll32.exe" || len(open.Args) != 2 || open.Args[0] != "url.dll,FileProtocolHandler" || open.Args[1] != path {
		t.Fatalf("unexpected open command: %#v", open)
	}

	reveal := revealVideoCommand(path)
	if reveal.Name != "explorer.exe" || len(reveal.Args) != 2 || reveal.Args[0] != `/select,` || reveal.Args[1] != path {
		t.Fatalf("unexpected reveal command: %#v", reveal)
	}
}
