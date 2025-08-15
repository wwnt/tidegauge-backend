package connWrap

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"
)

type ConnCommon interface {
	io.ReadWriter
	ResetInputBuffer() (err error)
}

type ConnUtil struct {
	ConnCommon
	sync.Mutex
	Typ string
}

func NewConnUtil(conn ConnCommon) *ConnUtil {
	return &ConnUtil{
		ConnCommon: conn,
	}
}

func (c *ConnUtil) writeCommand(input []byte) (n int, err error) {
	// may be the previous command is not finished
	time.Sleep(500 * time.Millisecond)
	if err = c.ResetInputBuffer(); err != nil {
		return 0, err
	}
	return c.Write(input)
}

func (c *ConnUtil) ReadLine(input []byte) (line string, err error) {
	defer c.UnlockCheckNotTimeout(err)
	c.Lock()

	_, err = c.writeCommand(input)
	if err != nil {
		return "", err
	}
	line, err = bufio.NewReader(c).ReadString('\n')
	return line, err
}

func (c *ConnUtil) Scan(input []byte, outputF string, v ...any) (err error) {
	defer c.UnlockCheckNotTimeout(err)
	c.Lock()
	_, err = c.writeCommand(input)
	if err != nil {
		return &Error{Type: ErrIO, Send: input, Err: err}
	}
	_, err = fmt.Fscanf(c, outputF, v...)
	if err != nil {
		return &Error{Type: ErrParse, Send: input, Err: err}
	}
	return nil
}

func (c *ConnUtil) CustomCommand(input []byte) (received []byte, err error) {
	defer c.UnlockCheckNotTimeout(err)
	c.Lock()

	if _, err = c.writeCommand(input); err != nil {
		return nil, err
	}
	time.Sleep(2 * time.Second)
	var buf = make([]byte, 100)
	var n int
	for {
		n, err = c.Read(buf)
		if err != nil {
			break
		}
		if n > 0 {
			received = append(received, buf[:n]...)
		}
	}
	return received, err
}

func (c *ConnUtil) UnlockCheckNotTimeout(err error) {
	if err != nil && !errors.Is(err, ErrTimeout) {
		time.Sleep(time.Second) // wait for end
	}
	c.Unlock()
}

const arduinoCommandEnd = '\xFF'

func (c *ConnUtil) SDI12ConcurrentMeasurement(addr string, extraWakeTime byte, output string, wait time.Duration) error {
	var input = []byte(addr + "C!")
	if c.Typ == "arduino" {
		// additional Bytes for arduino
		input = append(input, extraWakeTime, arduinoCommandEnd)
	}

	if err := c.Scan(input, output); err != nil {
		return err
	}
	time.Sleep(wait)
	return nil
}

func (c *ConnUtil) GetSDI12Data(addr string, extraWakeTime byte, resultsExpected int) (values []*float64, err error) {
	defer c.UnlockCheckNotTimeout(err)
	c.Lock()

	var resultsReceived, cmdNumber int

	reader := bufio.NewReader(c)
	for resultsReceived < resultsExpected && cmdNumber <= 9 {
		input := []byte(addr + "D" + strconv.Itoa(cmdNumber) + "!")
		if c.Typ == "arduino" {
			input = append(input, extraWakeTime, arduinoCommandEnd)
		}
		if _, err = c.writeCommand(input); err != nil {
			return nil, &Error{Type: ErrIO, Err: err}
		}
		if _, err = reader.Discard(1); err != nil { // sensor address
			return nil, &Error{Type: ErrIO, Err: err}
		}
		var resultsInResp int
		for {
			// 1+0.01+0.000
			if bs, err := reader.Peek(1); err != nil {
				return nil, &Error{Type: ErrIO, Err: err}
			} else if bs[0] == '\r' {
				if _, err = reader.Discard(2); err != nil {
					return nil, &Error{Type: ErrIO, Err: err}
				}
				if resultsInResp == 0 { // does not contain data
					return nil, &Error{Type: ErrIO, Err: errors.New("wrong number of data")}
				}
				break
			}
			var f float64
			if _, err = fmt.Fscan(reader, &f); err != nil {
				return nil, &Error{Type: ErrParse, Send: input, Err: err}
			}
			values = append(values, &f)
			resultsReceived++
			resultsInResp++
		}
		cmdNumber++
	}
	if resultsReceived != resultsExpected {
		return nil, &Error{Type: ErrParse, Err: errors.New("wrong number of data")}
	}
	return values, nil
}

func (c *ConnUtil) AnalogRead(pin byte) (int, error) {
	if c.Typ != "arduino" {
		return 0, errors.New("only for arduino")
	}
	var output = "%d\r\n"
	var val int
	err := c.Scan([]byte{pin, arduinoCommandEnd}, output, &val)
	return val, err
}
