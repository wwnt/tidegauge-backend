package arduino

import (
	"fmt"
	"tide/tide_client/connWrap"
)

const commandEnd byte = 0xFF

type Session struct {
	bus *connWrap.Bus
}

func NewSession(bus *connWrap.Bus) *Session {
	return &Session{bus: bus}
}

func (s *Session) AnalogRead(pin byte) (val int, err error) {
	input := []byte{pin, commandEnd}
	s.bus.Lock()
	defer s.bus.Unlock()

	if _, writeErr := s.bus.Write(input); writeErr != nil {
		err = &connWrap.Error{Type: connWrap.ErrIO, Send: input, Err: writeErr}
		return 0, err
	}
	if _, scanErr := fmt.Fscanf(s.bus, "%d\r\n", &val); scanErr != nil {
		err = &connWrap.Error{Type: connWrap.ErrParse, Send: input, Err: scanErr}
		return 0, err
	}
	return val, nil
}
