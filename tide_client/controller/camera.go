package controller

import (
	"log/slog"
	"os"
	"path"
	"tide/tide_client/global"
	"time"
)

func scheduleRemoveCameraOutdated() {
	if global.Config.Cameras.Ftp.Path != "" {
		removeCameraOutdatedFileJob := func() { cleanDir(global.Config.Cameras.Ftp.Path, global.Config.Cameras.Ftp.HoldDays, 0) }
		removeCameraOutdatedFileJob()
		if _, err := global.CronJob.AddFunc("@daily", removeCameraOutdatedFileJob); err != nil {
			global.Log.Fatal(err)
		}
	}
}

func cleanDir(parentPath string, maxAge time.Duration, depth int) {
	expireTime := time.Now().Add(-maxAge)
	entries, err := os.ReadDir(parentPath)
	if err != nil {
		slog.Error("read dir error", err)
		return
	}
	if len(entries) == 0 && depth > 1 {
		_ = os.Remove(parentPath)
	}
	for _, entry := range entries {
		fullPath := path.Join(parentPath, entry.Name())
		if entry.IsDir() {
			cleanDir(fullPath, maxAge, depth+1)
		} else {
			fileInfo, err := entry.Info()
			if err != nil {
				slog.Error("read file info error", err)
				return
			}
			if fileInfo.ModTime().Before(expireTime) {
				if err = os.Remove(fullPath); err != nil {
					slog.Error("remove file error", err)
				}
			}
		}
	}
}
