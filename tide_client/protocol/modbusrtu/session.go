package modbusrtu

import (
	"github.com/wwnt/modbus"

	"tide/tide_client/connWrap"
)

type Session struct {
	bus    *connWrap.Bus
	client modbus.Client
}

func NewSession(bus *connWrap.Bus, slaveID byte) *Session {
	handler := modbus.NewRTUClientHandler(bus)
	handler.SlaveId = slaveID
	handler.BaudRate = bus.SerialBaudRate()

	return &Session{
		bus:    bus,
		client: modbus.NewClient(handler),
	}
}

func (s *Session) ReadInputRegisters(address, quantity uint16) (result []byte, err error) {
	s.bus.Lock()
	defer s.bus.Unlock()

	return s.client.ReadInputRegisters(address, quantity)
}

func (s *Session) ReadHoldingRegisters(address, quantity uint16) (result []byte, err error) {
	s.bus.Lock()
	defer s.bus.Unlock()

	return s.client.ReadHoldingRegisters(address, quantity)
}
