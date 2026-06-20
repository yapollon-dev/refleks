//go:build !windows

package recording

import "errors"

func openVideoPath(string) error {
	return errors.New("opening recordings is only supported on Windows")
}

func revealVideoPath(string) error {
	return errors.New("revealing recordings is only supported on Windows")
}
