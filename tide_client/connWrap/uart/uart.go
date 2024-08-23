package uart

import (
	"errors"
	"go.bug.st/serial"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"tide/tide_client/connWrap"
	"time"
)

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

type Mode struct {
	BaudRate int    `json:"baud_rate"` // The serial port bitrate (aka Baud rate)
	DataBits int    `json:"data_bits"` // Size of the character (must be 5, 6, 7 or 8)
	Parity   string `json:"parity"`    // Parity: None, Odd, Even, Mark, Space
}

type Uart struct {
	mode        serial.Mode
	portName    string
	conn        serial.Port
	readTimeout time.Duration
	inReconnect atomic.Bool
}

func NewUart(name string, readTimeout uint32, mode Mode) (connWrap.ConnCommon, error) {
	c := &Uart{
		portName:    name,
		conn:        nil,
		readTimeout: time.Duration(readTimeout) * time.Millisecond,
		mode: serial.Mode{
			BaudRate: mode.BaudRate,
			DataBits: mode.DataBits,
			Parity:   getParity(mode.Parity),
			StopBits: serial.OneStopBit,
		},
	}
	go c.reopenUntilSuccess()
	return c, nil
}

func (c *Uart) reopenUntilSuccess() {
	if !c.inReconnect.CompareAndSwap(false, true) {
		// inReconnect == true
		return
	}
	defer c.inReconnect.Store(false)
	if c.conn != nil {
		_ = c.conn.Close()
	}
	var err error
	for {
		if err = c.open(); err != nil {
			log.Printf("connect to %s failed: %s\n", c.portName, err)
		} else {
			log.Printf("connected to %s", c.portName)
			break
		}
		time.Sleep(10 * time.Second)
	}
}

func (c *Uart) open() error {
	port, err := serial.Open(c.portName, &c.mode)
	if err == nil {
		if err = port.SetReadTimeout(c.readTimeout); err == nil {
			c.conn = port
		}
	}
	return err
}

func (c *Uart) Read(b []byte) (n int, err error) {
	if c.conn == nil {
		return 0, os.ErrInvalid
	}
	defer func() { c.handleErr(err) }()
	n, err = c.conn.Read(b)
	if n == 0 && err == nil {
		return 0, connWrap.ErrTimeout
	}
	return n, err
}

func (c *Uart) Write(b []byte) (n int, err error) {
	if c.conn == nil {
		return 0, os.ErrInvalid
	}
	defer func() { c.handleErr(err) }()
	return c.conn.Write(b)
}

func (c *Uart) ResetInputBuffer() (err error) {
	if c.conn == nil {
		return os.ErrInvalid
	}
	defer func() { c.handleErr(err) }()
	return c.conn.ResetInputBuffer()
}

func (c *Uart) handleErr(err error) {
	if err == nil || errors.Is(err, connWrap.ErrTimeout) {
		return
	}
	go c.reopenUntilSuccess()
}
