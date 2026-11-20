package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path"
	"strings"
	"tide/pkg"
	"time"

	"go.bug.st/serial"
)

func main() {
	// 所有致命错误都以非零状态退出：服务单元是 Restart=on-failure，
	// 以 0 退出（例如启动时监听地址还没就绪）systemd 不会再拉起进程。
	if err := run(); err != nil {
		log.Println(err)
		os.Exit(1)
	}
}

func run() error {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	err := os.MkdirAll("log", 0751)
	if err != nil {
		return err
	}
	var ts = time.Now().Format("2006-01-02__15-04-05")
	logF, err := os.Create(path.Join("log", ts+".log"))
	if err != nil {
		return err
	}
	multiWriter := io.MultiWriter(os.Stdout, logF)
	log.SetOutput(multiWriter)

	addr := flag.String("l", ":7000", "listen tcp addr. Check https://pkg.go.dev/net#Listen")
	portName := flag.String("s", "/dev/ttyUSB0", "serial port name")
	baudRate := flag.Int("r", 9600, "baud rate")
	dataBits := flag.Int("d", 8, "data bits")
	parity := flag.String("parity", "None", "parity (None, Odd, Even)")
	debug := flag.Bool("debug", false, "print data")

	flag.Parse()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	log.Println("listen on:", *addr)

	for {
		tcpConn, err := ln.Accept()
		if err != nil {
			return err
		}
		log.Println("connected from:", tcpConn.RemoteAddr())

		var uartConn serial.Port
		for {
			uartConn, err = openSerial(*portName, *baudRate, *dataBits, *parity)
			if err != nil {
				log.Println(err)
				time.Sleep(5 * time.Second)
			} else {
				log.Println("connected to:", *portName)
				break
			}
		}

		// 连接和串口按参数传入：每个会话的 goroutine 只操作自己的那一对。
		// 以前闭包捕获的是循环外的变量，上一个会话收尾时可能关掉下一个会话的连接。
		go func(tcpConn net.Conn, uartConn serial.Port) {
			var buf = make([]byte, 100)
			for {
				n, err := uartConn.Read(buf)
				if err != nil {
					log.Println(err)
					_ = uartConn.Close()
					_ = tcpConn.Close()
					break
				}
				if *debug {
					_, _ = fmt.Fprint(multiWriter, string(buf[:n]))
				}
				_, err = tcpConn.Write(buf[:n])
				if err != nil {
					log.Println(err)
				}
			}
		}(tcpConn, uartConn)

		var buf = make([]byte, 100)
		for {
			n, err := tcpConn.Read(buf)
			if err != nil {
				log.Println(err)
				_ = uartConn.Close()
				_ = tcpConn.Close()
				break
			}

			if *debug {
				log.Println(string(pkg.Printable(buf[:n])))
			}

			_, err = uartConn.Write(buf[:n])
			if err != nil {
				log.Println(err)
			}
		}
	}
}

func openSerial(portName string, baudRate int, dataBits int, parity string) (serial.Port, error) {
	port, err := serial.Open(portName, &serial.Mode{
		BaudRate: baudRate,
		DataBits: dataBits,
		Parity:   getParity(parity),
		StopBits: serial.OneStopBit,
	})
	return port, err
}

func getParity(parity string) serial.Parity {
	parity = strings.ToLower(parity)
	if p, ok := parityMap[parity]; ok {
		return p
	} else {
		return serial.NoParity
	}
}

var parityMap = map[string]serial.Parity{
	"none":  serial.NoParity,
	"odd":   serial.OddParity,
	"even":  serial.EvenParity,
	"mark":  serial.MarkParity,
	"space": serial.SpaceParity,
}
