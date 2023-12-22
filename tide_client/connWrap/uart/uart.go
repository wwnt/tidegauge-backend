package uart

import (
	"errors"
	"github.com/albenik/go-serial/v2"
	"sync/atomic"
	"tide/tide_client/connWrap"
	"tide/tide_client/global"
	"time"
)

type Mode struct {
	BaudRate int           `json:"baud_rate"` // The serial port bitrate (aka Baud rate)
	DataBits int           `json:"data_bits"` // Size of the character (must be 5, 6, 7 or 8)
	Parity   serial.Parity `json:"parity"`    // Parity: N - None, E - Even, O - Odd
}

type Uart struct {
	Mode
	portName    string
	conn        *serial.Port
	readTimeout uint32
	inReconnect atomic.Bool
}

func NewUart(name string, readTimeout uint32, m Mode) (connWrap.ConnCommon, error) {
	p := &Uart{portName: name, readTimeout: readTimeout, Mode: m}
	p.reopenUntilSuccess()
	return p, nil
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
	go func() {
		var err error
		for {
			if err = c.open(); err == nil {
				break
			} else {
				global.Log.Errorf("open %s: %s", c.portName, err)
			}
			time.Sleep(10 * time.Second)
		}
	}()
}

func (c *Uart) open() error {
	port, err := serial.Open(c.portName,
		serial.WithBaudrate(c.BaudRate),
		serial.WithParity(c.Parity),
		serial.WithDataBits(c.DataBits),
		serial.WithStopBits(serial.OneStopBit),
		serial.WithHUPCL(true), // make sure to reset Arduino
	)
	if err == nil {
		if err = port.SetFirstByteReadTimeout(c.readTimeout); err == nil {
			c.conn = port
		}
	}
	return err
}

func (c *Uart) Read(b []byte) (n int, err error) {
	defer c.handleErr(err)
	n, err = c.conn.Read(b)
	if n == 0 && err == nil {
		return 0, connWrap.ErrTimeout
	}
	return n, err
}

func (c *Uart) Write(b []byte) (n int, err error) {
	defer c.handleErr(err)
	return c.conn.Write(b)
}

func (c *Uart) ReadyToRead() (n uint32, err error) {
	defer c.handleErr(err)
	return c.conn.ReadyToRead()
}

func (c *Uart) ResetInputBuffer() (err error) {
	defer c.handleErr(err)
	return c.conn.ResetInputBuffer()
}

func (c *Uart) handleErr(err error) {
	if err == nil || errors.Is(err, connWrap.ErrTimeout) {
		return
	}
	c.reopenUntilSuccess()
}
