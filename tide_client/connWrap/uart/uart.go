package uart

import (
	"errors"
	"github.com/albenik/go-serial/v2"
	"tide/tide_client/connWrap"
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
}

func NewUart(name string, readTimeout uint32, m Mode) (connWrap.ConnCommon, error) {
	p := &Uart{portName: name, readTimeout: readTimeout, Mode: m}
	err := p.open()
	return p, err
}

func (c *Uart) open() error {
	if c.conn != nil {
		_ = c.conn.Close()
	}
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
	n, err = c.conn.Read(b)
	if n == 0 && err == nil {
		return 0, connWrap.ErrTimeout
	}
	if err != nil {
		if err2 := c.open(); err2 != nil {
			err = errors.New(err.Error() + ". Reopen: " + err2.Error())
		}
	}
	return n, err
}

func (c *Uart) Write(b []byte) (n int, err error) {
	if err = c.ResetInputBuffer(); err != nil {
		return 0, err
	}
	n, err = c.conn.Write(b)
	if err != nil {
		if err2 := c.open(); err2 != nil {
			err = errors.New(err.Error() + ". Reopen: " + err2.Error())
		}
	}
	return n, err
}

func (c *Uart) Close() error {
	return c.conn.Close()
}

func (c *Uart) ReadyToRead() (uint32, error) {
	return c.conn.ReadyToRead()
}

func (c *Uart) ResetInputBuffer() (err error) {
	return c.conn.ResetInputBuffer()
}
