package main

import (
	"flag"
	"fmt"
	"go.bug.st/serial"
	"io"
	"log"
	"net"
	"os"
	"path"
	"strings"
	"tide/pkg"
	"time"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	var ts = time.Now().Format("2006-01-02__15-04-05")
	logF, err := os.Create(path.Join("log", ts+".log"))
	if err != nil {
		log.Println(err)
		return
	}
	log.SetOutput(io.MultiWriter(os.Stdout, logF))

	addr := flag.String("l", ":7000", "listen tcp addr")
	portName := flag.String("port", "/dev/ttyUSB0", "serial port name")
	baudRate := flag.Int("r", 9600, "baud rate")
	dataBits := flag.Int("d", 8, "data bits")
	parity := flag.String("p", "None", "parity (None, Odd, Even)")
	debug := flag.Bool("debug", false, "print data")

	flag.Parse()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Println(err)
		return
	}
	log.Println("listen on:", *addr)

	var tcpConnNew net.Conn
	var uartConn serial.Port
	for {
		tcpConnNew, err = ln.Accept()
		if err != nil {
			log.Println(err)
			return
		}
		log.Println("connected from:", tcpConnNew.RemoteAddr())

		for {
			uartConn, err = openSerial(portName, baudRate, dataBits, parity)
			if err != nil {
				log.Println(err)
				time.Sleep(5 * time.Second)
			} else {
				log.Println("connected to:", *portName)
				break
			}
		}

		go func() {
			var buf = make([]byte, 100)
			for {
				n, err := uartConn.Read(buf)
				if err != nil {
					log.Println(err)
					_ = uartConn.Close()
					_ = tcpConnNew.Close()
					break
				}
				if *debug {
					fmt.Print(string(buf[:n]))
				}
				_, err = tcpConnNew.Write(buf[:n])
				if err != nil {
					log.Println(err)
				}
			}
		}()

		var buf = make([]byte, 100)
		for {
			n, err := tcpConnNew.Read(buf)
			if err != nil {
				log.Println(err)
				_ = uartConn.Close()
				_ = tcpConnNew.Close()
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

func openSerial(portName *string, baudRate *int, dataBits *int, parity *string) (serial.Port, error) {
	port, err := serial.Open(*portName, &serial.Mode{
		BaudRate: *baudRate,
		DataBits: *dataBits,
		Parity:   selectParity(*parity),
		StopBits: serial.OneStopBit,
	})
	if err == nil {
		// reset arduino
		_ = port.SetDTR(true)
		time.Sleep(10 * time.Millisecond)
		_ = port.SetDTR(false)
		time.Sleep(time.Second)
	}
	return port, err
}

func selectParity(parity string) serial.Parity {
	parity = strings.ToLower(parity)
	switch parity {
	case "none":
		return serial.NoParity
	case "odd":
		return serial.OddParity
	case "even":
		return serial.EvenParity
	default:
		return serial.NoParity
	}
}
