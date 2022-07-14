package controller

import (
	"os"
	"path"
	"tide/tide_client/global"
	"time"
)

func scheduleRemoveCameraOutdated() {
	if global.Config.Cameras.Ftp.Path != "" {
		removeCameraOutdatedFileJob := func() { recursiveDir(global.Config.Cameras.Ftp.Path, removeCameraOutdatedFile) }
		removeCameraOutdatedFileJob()
		if _, err := global.CronJob.AddFunc("@daily", removeCameraOutdatedFileJob); err != nil {
			global.Log.Fatal(err)
		}
	}
}

func removeCameraOutdatedFile(parentPath string, dir os.DirEntry) (retOk bool) {
	if dir.IsDir() {
		if dir.Name()[0] == '.' { //ignore
			return
		}
		if t, err := time.Parse("2006-01-02", dir.Name()); err == nil {
			if t.Before(global.CameraHoldTime) {
				if err = os.RemoveAll(path.Join(parentPath, dir.Name())); err != nil {
					global.Log.Error(err)
				}
			}
			return
		}
		return true
	} else {
		if dir.Name() == "DVRWorkDirectory" {
			return
		}
		fileInfo, err := dir.Info()
		if err != nil {
			global.Log.Error(err)
			return
		}
		if fileInfo.ModTime().Before(global.CameraHoldTime) {
			if err = os.Remove(path.Join(parentPath, dir.Name())); err != nil {
				global.Log.Error(err)
			}
		}
		return
	}
}

func recursiveDir(parentPath string, handleFunc func(parentPath string, dir os.DirEntry) bool) {
	dirs, err := os.ReadDir(parentPath)
	if err != nil {
		global.Log.Error(err)
		return
	}
	if len(dirs) == 0 && parentPath != global.Config.Cameras.Ftp.Path {
		_ = os.Remove(parentPath)
	}
	for _, dir := range dirs {
		if handleFunc(parentPath, dir) {
			recursiveDir(path.Join(parentPath, dir.Name()), handleFunc)
		}
	}
}
