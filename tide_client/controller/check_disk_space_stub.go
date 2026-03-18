//go:build !linux

package controller

import "errors"

// CheckDiskSpace checks the available disk space for a given path.
func CheckDiskSpace(string) (uint64, error) {
	return 0, errors.New("disk space check is only supported on linux")
}
