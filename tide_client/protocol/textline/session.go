package textline

import (
	"bufio"
	"errors"
	"fmt"
	"time"

	"tide/tide_client/connWrap"
)

type Session struct {
	bus               *connWrap.Bus
	customCommandWait time.Duration
}

func NewSession(bus *connWrap.Bus) *Session {
	return &Session{
		bus:               bus,
		customCommandWait: time.Second,
	}
}

func (s *Session) ReadLine(input []byte) (line string, err error) {
	s.bus.Lock()
	defer s.bus.Unlock()

	if _, err = s.bus.Write(input); err != nil {
		return "", err
	}
	line, err = bufio.NewReader(s.bus).ReadString('\n')
	return line, err
}

func (s *Session) Scan(input []byte, outputF string, v ...any) (err error) {
	s.bus.Lock()
	defer s.bus.Unlock()

	if _, writeErr := s.bus.Write(input); writeErr != nil {
		return &connWrap.Error{Type: connWrap.ErrIO, Send: input, Err: writeErr}
	}
	if _, scanErr := fmt.Fscanf(s.bus, outputF, v...); scanErr != nil {
		return &connWrap.Error{Type: connWrap.ErrParse, Send: input, Err: scanErr}
	}
	return nil
}

func (s *Session) CustomCommand(input []byte) (received []byte, err error) {
	s.bus.Lock()
	defer s.bus.Unlock()

	if _, err = s.bus.Write(input); err != nil {
		return nil, err
	}
	time.Sleep(s.customCommandWait)

	buf := make([]byte, 100)
	for {
		n, readErr := s.bus.Read(buf)
		if n > 0 {
			received = append(received, buf[:n]...)
		}
		if readErr != nil {
			if errors.Is(readErr, connWrap.ErrTimeout) && len(received) > 0 {
				return received, nil
			}
			return received, readErr
		}
	}
}
