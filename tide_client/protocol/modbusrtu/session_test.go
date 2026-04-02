package modbusrtu

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/wwnt/modbus"

	"tide/tide_client/connWrap"
)

type fakeSerialConn struct {
	baudRate  int
	lastWrite []byte
	response  []byte
	writeAt   time.Time
}

func (c *fakeSerialConn) Read(p []byte) (int, error) {
	if len(c.response) == 0 {
		return 0, io.EOF
	}
	n := copy(p, c.response)
	c.response = c.response[n:]
	return n, nil
}

func (c *fakeSerialConn) Write(p []byte) (int, error) {
	c.lastWrite = append([]byte(nil), p...)
	c.writeAt = time.Now()

	handler := modbus.NewRTUClientHandler(bytes.NewBuffer(nil))
	handler.SlaveId = p[0]
	resp, err := handler.Encode(&modbus.ProtocolDataUnit{
		FunctionCode: modbus.FuncCodeReadInputRegisters,
		Data:         []byte{0x02, 0x12, 0x34},
	})
	if err != nil {
		return 0, err
	}
	c.response = resp
	return len(p), nil
}

func (c *fakeSerialConn) ResetInputBuffer() error { return nil }

func (c *fakeSerialConn) SerialBaudRate() int { return c.baudRate }

func TestSessionReadInputRegisters(t *testing.T) {
	t.Parallel()

	rawConn := &fakeSerialConn{baudRate: 115200}
	bus := connWrap.NewBus(rawConn)
	session := NewSession(bus, 0x11)

	got, err := session.ReadInputRegisters(2, 1)
	if err != nil {
		t.Fatalf("ReadInputRegisters returned error: %v", err)
	}
	if !bytes.Equal(got, []byte{0x12, 0x34}) {
		t.Fatalf("ReadInputRegisters = %v, want %v", got, []byte{0x12, 0x34})
	}

	wantReq := []byte{0x11, modbus.FuncCodeReadInputRegisters, 0x00, 0x02, 0x00, 0x01}
	if !bytes.Equal(rawConn.lastWrite[:6], wantReq) {
		t.Fatalf("request prefix = %v, want %v", rawConn.lastWrite[:6], wantReq)
	}
}

func TestSessionWaitsQuietTimeBeforeRequest(t *testing.T) {
	t.Parallel()

	const quietTime = 50 * time.Millisecond

	rawConn := &fakeSerialConn{baudRate: 115200}
	bus := connWrap.NewBusWithQuietTime(rawConn, quietTime)
	session := NewSession(bus, 0x11)

	start := time.Now()
	if _, err := session.ReadInputRegisters(2, 1); err != nil {
		t.Fatalf("ReadInputRegisters returned error: %v", err)
	}

	if rawConn.writeAt.IsZero() {
		t.Fatal("request was not written")
	}

	const tolerance = 10 * time.Millisecond
	if delay := rawConn.writeAt.Sub(start); delay < quietTime-tolerance {
		t.Fatalf("request sent after %v, want at least %v", delay, quietTime)
	}
}
