//go:build linux

package controller

import "syscall"

// CheckDiskSpace checks the available disk space for a given path.
func CheckDiskSpace(path string) (uint64, error) {
	var stat syscall.Statfs_t
	err := syscall.Statfs(path, &stat)
	if err != nil {
		return 0, err
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}
