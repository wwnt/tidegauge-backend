package main

import (
	"encoding/json"
	"github.com/rjeczalik/notify"
	"io"
	"io/fs"
	"log"
	"net"
	"os"
	"path"
	"path/filepath"
	"tide/common"
	"tide/tide_client/global"
	"time"
)

type fileInfo struct {
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}
type writeFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

func _walkAndWatchFtp(conn net.Conn) {
	var remoteFiles map[string]fileInfo
	err := json.NewDecoder(conn).Decode(&remoteFiles)
	if err != nil {
		log.Println(err)
		return
	}
	encoder := json.NewEncoder(conn)
	err = filepath.WalkDir(global.Config.Cameras.Ftp.Path, func(localPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			switch path.Ext(d.Name()) {
			case ".jpg", ".dav":
				localInfo, err := d.Info()
				if err != nil {
					return err
				}
				remoteInfo, ok := remoteFiles[localPath]
				if ok { //续传
					if remoteInfo.Size == localInfo.Size() && remoteInfo.ModTime == localInfo.ModTime() {
						break
					}
				}
				err = encoder.Encode(writeFile{Path: localPath, Size: localInfo.Size()})
				if err != nil {
					log.Println(err)
					return err
				}
				localFile, err := os.Open(path.Join(global.Config.Cameras.Ftp.Path, localPath))
				if err != nil {
					return err
				}
				_, _ = io.CopyN(conn, localFile, localInfo.Size())
			}
		}
		return err
	})
	if err != nil {
		log.Println(err)
		return
	}
	err = encoder.Encode(remoteFiles)
	if err != nil {
		return
	}
	// Make the channel buffered to ensure no event is dropped. Notify will drop
	// an event if the receiver is not able to keep up the sending pace.
	c := make(chan notify.EventInfo, 100)

	// Set up a watchpoint listening for events within a directory tree rooted
	// at current working directory. Dispatch remove events to c.
	if err = notify.Watch("./...", c, notify.InCloseWrite|notify.InMovedTo); err != nil {
		log.Fatal(err)
	}
	defer notify.Stop(c)
	select {

	case evtInfo := <-c:
		switch path.Ext(evtInfo.Path()) {
		case ".jpg":
			file, err := os.Open(evtInfo.Path())
			if err != nil {
				log.Println(err)
				break
			}
			info, err := file.Stat()
			if err != nil {
				log.Println(err)
				break
			}
			//conn, err := session.Open()
			if err != nil {
				log.Println(err)
				return
			}
			if _, err := conn.Write([]byte{common.MsgTypeSyncFile}); err != nil {
				log.Println(err)
				return
			}
			err = json.NewEncoder(conn).Encode(fileInfo{ModTime: info.ModTime()})
			if err != nil {
				log.Println(err)
				return
			}
			_, err = io.Copy(conn, file)
			if err != nil {
				log.Println(err)
			}
		case ".dav":
			log.Println(evtInfo)
		}
	}
}
