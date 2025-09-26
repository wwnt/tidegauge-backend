package project

import (
	"errors"
	"io"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

func Run(start, stop func(), aborted func(time.Time)) {
	var err error
	const lastActive = "lastActive"
	var lastActiveFile *os.File
	if lastActiveFile, err = os.OpenFile(lastActive, os.O_RDWR, 0); err == nil {
		//lastActive exist
		b, err := io.ReadAll(lastActiveFile)
		if err != nil {
			log.Fatal(err)
		}
		ts, err := strconv.ParseInt(string(b), 10, 0)
		if err != nil {
			log.Fatal(err)
		}
		aborted(time.Unix(ts, 0))
	} else if errors.Is(err, fs.ErrNotExist) {
		// create lastActive
		if lastActiveFile, err = os.Create(lastActive); err != nil {
			log.Fatal(err)
		}
	} else {
		log.Fatal(err)
	}
	go func() {
		for {
			if _, err := lastActiveFile.WriteAt([]byte(strconv.FormatInt(time.Now().Unix(), 10)), 0); err != nil {
				log.Println(err)
				_ = lastActiveFile.Close()
				return
			}
			time.Sleep(30 * time.Second)
		}
	}()

	start()
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, syscall.SIGINT)

	<-c
	stop()

	_ = lastActiveFile.Close()
	if err = os.Remove(lastActive); err != nil {
		log.Println(err)
	}
}

var releaseJobs []func()

func RegisterReleaseFunc(f func()) {
	releaseJobs = append(releaseJobs, f)
}

func CallReleaseFunc() {
	for i := len(releaseJobs) - 1; i >= 0; i-- {
		releaseJobs[i]()
	}
}
