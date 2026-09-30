package controller

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
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

// scheduleRemoveOutdatedData 装订磁盘与数据库的保留期清理：启动即跑一次，之后每日一次。
// dataItemNames 是本站全部数据项（即 SQLite 数据表名）。
func scheduleRemoveOutdatedData(dataItemNames []string) {
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
	// data.db 所在盘也在监控范围内：时序数据与状态日志是主要的磁盘增长源，
	// 而纯数据站（未配置 cameras/gnss）原本没有任何磁盘余量检查。
	dbDir, dbDirOk := db.PathFromDSN(global.Config.Db.Dsn)

	removeOutdatedDataJob := func() {
		for _, ftp := range validFtps {
			cleanDir(ftp.Path, 1, 0, time.Now().Add(-time.Duration(ftp.HoldDays)*24*time.Hour))
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
		if dbDirOk {
			if avail, err := CheckDiskSpace(dbDir); err != nil {
				slog.Error("Failed to check disk space", "path", dbDir, "error", err)
				os.Exit(1)
			} else if avail < minDiskThreshold {
				slog.Error("Insufficient disk space",
					"path", dbDir,
					"available_mb", avail/(1024*1024),
					"threshold_mb", minDiskThreshold/(1024*1024))
				os.Exit(1)
			}
		}
		if err := db.CleanDBData(time.Now().Add(-time.Duration(global.Config.Db.HoldDays)*24*time.Hour).UnixMilli(), dataItemNames); err != nil {
			slog.Error("Failed to clean outdated database data", "error", err)
		}
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
				slog.Info("Deleted empty directory", "path", parentPath)
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
