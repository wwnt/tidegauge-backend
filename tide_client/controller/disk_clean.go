package controller

import (
	"fmt"
	"io"
	"log"
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
			log.Fatal(err)
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
				log.Fatalf("CheckDiskSpace error in %s: %s\n", ftp.Path, err)
			} else if avail < minDiskThreshold {
				log.Fatalf("Disk space is less than %dMB in %s\n", minDiskThreshold/(1024*1024), ftp.Path)
			}
		}
		db.CleanDBData(time.Now().Add(-global.Config.Db.HoldDays * 24 * time.Hour).Unix())
	}
	removeOutdatedDataJob()
	if _, err := global.CronJob.AddFunc("@daily", removeOutdatedDataJob); err != nil {
		log.Fatalf("cron.AddFunc error: %s\n", err)
	}
}

func cleanDir(parentPath string, maxDepth int, currentDepth int, cutoffTime time.Time) {
	entries, err := os.ReadDir(parentPath)
	if err != nil {
		log.Printf("failed to read directory %s: %v\n", parentPath, err)
		return
	}

	for _, entry := range entries {
		fullPath := filepath.Join(parentPath, entry.Name())

		if entry.IsDir() {
			cleanDir(fullPath, maxDepth, currentDepth+1, cutoffTime)
		} else {
			fileInfo, err := entry.Info()
			if err != nil {
				log.Printf("failed to get info for file %s: %v\n", fullPath, err)
				continue
			}
			if fileInfo.ModTime().Before(cutoffTime) {
				if err = os.Remove(fullPath); err != nil {
					log.Printf("failed to delete file %s: %v\n", fullPath, err)
				} else {
					log.Printf("delete file: %s\n", fullPath)
				}
			}
		}
	}
	if currentDepth > maxDepth {
		isEmpty, err := isDirEmpty(parentPath)
		if err != nil {
			log.Printf("failed to check if directory %s is empty: %v\n", parentPath, err)
		} else if isEmpty {
			if err = os.Remove(parentPath); err != nil {
				log.Printf("failed to delete empty directory %s: %v\n", parentPath, err)
			} else {
				log.Printf("deleted empty directory: %s\n", parentPath)
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
