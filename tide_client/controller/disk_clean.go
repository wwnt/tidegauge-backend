package controller

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
	"tide/tide_client/db"
	"tide/tide_client/global"
	"time"
)

const minDiskThreshold = 128 * 1024 * 1024

func CheckFtpConfig(ftp global.Ftp) (bool, error) {
	if ftp.Path == "" && ftp.HoldDays == 0 {
		return false, nil
	}
	if ftp.HoldDays < 3 {
		return false, fmt.Errorf("holdDays must >= 3")
	}
	return true, nil
}

func scheduleRemoveOutdatedData() {
	var ftps = []global.Ftp{global.Config.Cameras.Ftp, global.Config.Gnss.Ftp}
	var validFtps []global.Ftp
	for _, ftp := range ftps {
		ok, err := CheckFtpConfig(ftp)
		if err != nil {
			slog.Error("FTP config check failed", "error", err)
			os.Exit(1)
		} else if ok {
			validFtps = append(validFtps, ftp)
		}
	}
	removeOutdatedDataJob := func() {
		for _, ftp := range validFtps {
			cleanDir(ftp.Path, 1, 0, time.Now().Add(-ftp.HoldDays*24*time.Hour))
		}
		// Check free disk space
		for _, ftp := range validFtps {
			if avail, err := CheckDiskSpace(ftp.Path); err != nil {
				slog.Error("Failed to check disk space", "path", ftp.Path, "error", err)
				os.Exit(1)
			} else if avail < minDiskThreshold {
				slog.Error("Insufficient disk space",
					"path", ftp.Path,
					"available_mb", avail/(1024*1024),
					"threshold_mb", minDiskThreshold/(1024*1024))
				os.Exit(1)
			}
		}
		db.CleanDBData(time.Now().Add(-global.Config.Db.HoldDays * 24 * time.Hour).Unix())
	}
	removeOutdatedDataJob()
	if _, err := global.CronJob.AddFunc("@daily", removeOutdatedDataJob); err != nil {
		slog.Error("Failed to add cleanup cron job", "error", err)
		os.Exit(1)
	}
}

func cleanDir(parentPath string, maxDepth int, currentDepth int, cutoffTime time.Time) {
	entries, err := os.ReadDir(parentPath)
	if err != nil {
		slog.Error("Failed to read directory", "path", parentPath, "error", err)
		return
	}

	for _, entry := range entries {
		fullPath := filepath.Join(parentPath, entry.Name())

		if entry.IsDir() {
			cleanDir(fullPath, maxDepth, currentDepth+1, cutoffTime)
		} else {
			fileInfo, err := entry.Info()
			if err != nil {
				slog.Error("Failed to get file info", "path", fullPath, "error", err)
				continue
			}
			if fileInfo.ModTime().Before(cutoffTime) {
				if err = os.Remove(fullPath); err != nil {
					slog.Error("Failed to delete file", "path", fullPath, "error", err)
				} else {
					slog.Info("Deleted expired file", "path", fullPath)
				}
			}
		}
	}
	if currentDepth > maxDepth {
		isEmpty, err := isDirEmpty(parentPath)
		if err != nil {
			slog.Error("Failed to check if directory is empty", "path", parentPath, "error", err)
		} else if isEmpty {
			if err = os.Remove(parentPath); err != nil {
				slog.Error("Failed to delete empty directory", "path", parentPath, "error", err)
			} else {
				slog.Info("Feleted empty directory", "path", parentPath)
			}
		}
	}
}

// isDirEmpty checks whether dirPath is empty.
func isDirEmpty(dirPath string) (bool, error) {
	f, err := os.Open(dirPath)
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	_, err = f.Readdirnames(1)
	if err == io.EOF {
		return true, nil
	}
	return false, err
}

// CheckDiskSpace checks the available disk space for a given path.
func CheckDiskSpace(path string) (uint64, error) {
	var stat syscall.Statfs_t
	err := syscall.Statfs(path, &stat)
	if err != nil {
		return 0, err
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}
