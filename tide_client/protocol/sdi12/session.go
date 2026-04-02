package sdi12

import (
	"bufio"
	"errors"
	"fmt"
	"strconv"
	"time"

	"tide/tide_client/connWrap"
)

type Mode int

const (
	ModeNative Mode = iota
	ModeArduino
)

const arduinoCommandEnd byte = 0xFF

type Session struct {
	bus  *connWrap.Bus
	mode Mode
}

func NewSession(bus *connWrap.Bus, mode Mode) *Session {
	return &Session{bus: bus, mode: mode}
}

func (s *Session) unlockBus(err *error) {
	// A malformed SDI-12 reply may still be draining from the sensor after
	// parsing fails. Keep the bus reserved long enough for that tail to end
	// before another device starts its next exchange.
	if err != nil && *err != nil && !errors.Is(*err, connWrap.ErrTimeout) {
		time.Sleep(time.Second)
	}
	s.bus.Unlock()
}

func (s *Session) ConcurrentMeasurement(addr string, extraWakeTime byte, output string, wait time.Duration) (err error) {
	input := []byte(addr + "C!")
	if s.mode == ModeArduino {
		input = append(input, extraWakeTime, arduinoCommandEnd)
	}

	{
		s.bus.Lock()
		defer s.unlockBus(&err)

		if _, writeErr := s.bus.Write(input); writeErr != nil {
			err = &connWrap.Error{Type: connWrap.ErrIO, Send: input, Err: writeErr}
			return err
		}
		if _, scanErr := fmt.Fscanf(s.bus, output); scanErr != nil {
			err = &connWrap.Error{Type: connWrap.ErrParse, Send: input, Err: scanErr}
			return err
		}
	}
	time.Sleep(wait)
	return nil
}

func (s *Session) GetData(addr string, extraWakeTime byte, resultsExpected int) (values []*float64, err error) {
	s.bus.Lock()
	defer s.unlockBus(&err)

	var resultsReceived, cmdNumber int

	reader := bufio.NewReader(s.bus)
	for resultsReceived < resultsExpected && cmdNumber <= 9 {
		input := []byte(addr + "D" + strconv.Itoa(cmdNumber) + "!")
		if s.mode == ModeArduino {
			input = append(input, extraWakeTime, arduinoCommandEnd)
		}
		if _, err = s.bus.Write(input); err != nil {
			return nil, &connWrap.Error{Type: connWrap.ErrIO, Send: input, Err: err}
		}
		if _, err = reader.Discard(1); err != nil {
			return nil, &connWrap.Error{Type: connWrap.ErrIO, Err: err}
		}

		var resultsInResp int
		for {
			// 1+0.01+0.000
			if bs, err := reader.Peek(1); err != nil {
				return nil, &connWrap.Error{Type: connWrap.ErrIO, Err: err}
			} else if bs[0] == '\r' {
				if _, err = reader.Discard(2); err != nil {
					return nil, &connWrap.Error{Type: connWrap.ErrIO, Err: err}
				}
				if resultsInResp == 0 {
					return nil, &connWrap.Error{Type: connWrap.ErrIO, Err: errors.New("wrong number of data")}
				}
				break
			}

			var f float64
			if _, err = fmt.Fscan(reader, &f); err != nil {
				return nil, &connWrap.Error{Type: connWrap.ErrParse, Send: input, Err: err}
			}
			values = append(values, &f)
			resultsReceived++
			resultsInResp++
		}
		cmdNumber++
	}
	if resultsReceived != resultsExpected {
		return nil, &connWrap.Error{Type: connWrap.ErrParse, Err: errors.New("wrong number of data")}
	}
	return values, nil
}
