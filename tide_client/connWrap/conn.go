package connWrap

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"strconv"
	"sync"
	"time"
)

type ConnCommon interface {
	io.ReadWriter
	ReadyToRead() (n uint32, err error)
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

func (c *ConnUtil) ReadLine(input []byte) (line string, err error) {
	defer c.UnlockCheckNotTimeout(err)
	c.Lock()

	_, err = c.Write(input)
	if err != nil {
		return "", err
	}
	line, err = bufio.NewReader(c).ReadString('\n')
	return line, err
}

func (c *ConnUtil) Scan(wait time.Duration, input []byte, outputF string, v ...interface{}) (err error) {
	defer c.UnlockCheckNotTimeout(err)
	c.Lock()
	time.Sleep(wait)
	_, err = c.Write(input)
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

	if _, err = c.Write(input); err != nil {
		return nil, err
	}
	var buf = make([]byte, 100)
	time.Sleep(300 * time.Millisecond)
	for {
		n, err := c.Read(buf)
		if err != nil {
			return nil, err
		}
		received = append(received, buf[:n]...)
		time.Sleep(100 * time.Millisecond)
		readyN, err := c.ReadyToRead()
		if err != nil {
			return nil, err
		}
		if readyN <= 0 {
			break
		}
	}
	return received, err
}

func (c *ConnUtil) UnlockCheckNotTimeout(err error) {
	if err != nil && err != ErrTimeout {
		time.Sleep(time.Second) // wait for end
	}
	c.Unlock()
}

const arduinoCommandEnd = '\xFF'

func (c *ConnUtil) SDI12ConcurrentMeasurement(addr string, extraWakeTime byte, output string, wait time.Duration) error {
	var input = []byte(addr + "C!")
	log.Println(string(input))
	if c.Typ == "arduino" {
		input = append(input, extraWakeTime, arduinoCommandEnd)
	}

	if err := c.Scan(time.Second/2, input, output); err != nil {
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
		log.Println(string(input))
		if c.Typ == "arduino" {
			input = append(input, extraWakeTime, arduinoCommandEnd)
		}
		time.Sleep(time.Second / 2)
		if _, err = c.Write(input); err != nil {
			return nil, &Error{Type: ErrIO, Err: err}
		}
		if _, err = reader.Discard(1); err != nil { //sensor address
			return nil, &Error{Type: ErrIO, Err: err}
		}
		for {
			if bs, err := reader.Peek(1); err != nil {
				return nil, &Error{Type: ErrIO, Err: err}
			} else if bs[0] == '\r' {
				if _, err = reader.Discard(2); err != nil {
					return nil, &Error{Type: ErrIO, Err: err}
				}
				break
			}
			var f float64
			if _, err = fmt.Fscan(reader, &f); err != nil {
				return nil, &Error{Type: ErrParse, Send: input, Err: err}
			}
			values = append(values, &f)
			resultsReceived++
		}
		cmdNumber++
	}
	if resultsReceived != resultsExpected {
		return nil, &Error{Type: ErrParse, Err: errors.New("wrong number of data")}
	}
	return values, nil
}

func (c *ConnUtil) AnalogRead(pin byte) (int, error) {
	var output = "%d\r\n"
	var val int
	err := c.Scan(0, []byte{pin, arduinoCommandEnd}, output, &val)
	return val, err
}
