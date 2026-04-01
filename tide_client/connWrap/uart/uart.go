package uart

import (
	"errors"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"tide/tide_client/connWrap"
	"time"

	"go.bug.st/serial"
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
	connMu      sync.RWMutex
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
	if conn := c.loadConn(); conn != nil {
		// Closing the old port is expected to interrupt any blocked Read/Write.
		// Keep the closed port published until a replacement is ready so concurrent
		// callers fail with the underlying I/O error instead of racing with nil.
		_ = conn.Close()
	}
	var err error
	for {
		if err = c.open(); err != nil {
			slog.Error("Failed to connect to UART port", "port", c.portName, "error", err)
		} else {
			slog.Info("Successfully connected to UART port", "port", c.portName)
			break
		}
		time.Sleep(10 * time.Second)
	}
}

func (c *Uart) open() error {
	port, err := serial.Open(c.portName, &c.mode)
	if err != nil {
		return err
	}
	if err = port.SetReadTimeout(c.readTimeout); err != nil {
		_ = port.Close()
		return err
	}
	c.storeConn(port)
	return nil
}

func (c *Uart) SerialBaudRate() int {
	return c.mode.BaudRate
}

func (c *Uart) loadConn() serial.Port {
	// Readers operate on a stable snapshot so reconnect can swap the port
	// reference without exposing a transient nil to concurrent callers.
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	return c.conn
}

func (c *Uart) storeConn(conn serial.Port) {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	c.conn = conn
}

func (c *Uart) Read(b []byte) (n int, err error) {
	conn := c.loadConn()
	if conn == nil {
		return 0, os.ErrInvalid
	}
	defer func() { c.handleErr(err) }()
	n, err = conn.Read(b)
	if n == 0 && err == nil {
		return 0, connWrap.ErrTimeout
	}
	return n, err
}

func (c *Uart) Write(b []byte) (n int, err error) {
	conn := c.loadConn()
	if conn == nil {
		return 0, os.ErrInvalid
	}
	defer func() { c.handleErr(err) }()
	return conn.Write(b)
}

func (c *Uart) ResetInputBuffer() (err error) {
	conn := c.loadConn()
	if conn == nil {
		return os.ErrInvalid
	}
	defer func() { c.handleErr(err) }()
	return conn.ResetInputBuffer()
}

func (c *Uart) handleErr(err error) {
	if err == nil || errors.Is(err, connWrap.ErrTimeout) {
		return
	}
	// Any non-timeout I/O error means the port is no longer trustworthy.
	// Reopen in the background; the current caller will observe the original error.
	go c.reopenUntilSuccess()
}
