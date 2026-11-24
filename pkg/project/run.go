package project

import (
	"errors"
	"io"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
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
		// 文件是先创建、再由后台 goroutine 写入时间戳的：首次写入前断电或磁盘写满都会
		// 留下一个空文件。时间戳只用于一条提示日志，读不出来就按"上次异常退出、时间未知"
		// 继续启动，不能因此拒绝启动。
		ts, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
		if err != nil {
			log.Printf("%s is unreadable (%v); previous run aborted at an unknown time", lastActive, err)
			aborted(time.Time{})
		} else {
			aborted(time.Unix(ts, 0))
		}
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
	for _, releaseJob := range slices.Backward(releaseJobs) {
		releaseJob()
	}
}
