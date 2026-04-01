package uart

import (
	"errors"
	"testing"
	"time"

	"go.bug.st/serial"
)

var errFakeClosed = errors.New("fake port closed")

type fakePort struct {
	closed  chan struct{}
	readErr error
}

func newFakePort(readErr error) *fakePort {
	return &fakePort{
		closed:  make(chan struct{}),
		readErr: readErr,
	}
}

func (p *fakePort) SetMode(*serial.Mode) error { return nil }

func (p *fakePort) Read(_ []byte) (int, error) {
	select {
	case <-p.closed:
		return 0, p.readErr
	default:
		return 0, nil
	}
}

func (p *fakePort) Write(b []byte) (int, error) { return len(b), nil }

func (p *fakePort) Drain() error { return nil }

func (p *fakePort) ResetInputBuffer() error { return nil }

func (p *fakePort) ResetOutputBuffer() error { return nil }

func (p *fakePort) SetDTR(bool) error { return nil }

func (p *fakePort) SetRTS(bool) error { return nil }

func (p *fakePort) GetModemStatusBits() (*serial.ModemStatusBits, error) { return nil, nil }

func (p *fakePort) SetReadTimeout(time.Duration) error { return nil }

func (p *fakePort) Close() error {
	select {
	case <-p.closed:
	default:
		close(p.closed)
	}
	return nil
}

func (p *fakePort) Break(time.Duration) error { return nil }

func TestReadDuringReconnectUsesClosedPortSnapshot(t *testing.T) {
	oldPort := newFakePort(errFakeClosed)
	newPort := newFakePort(nil)

	u := &Uart{
		portName: "COM1",
		conn:     oldPort,
		mode: serial.Mode{
			BaudRate: 9600,
		},
	}

	_ = oldPort.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		time.Sleep(50 * time.Millisecond)
		u.storeConn(newPort)
	}()

	var buf [1]byte
	if _, err := u.Read(buf[:]); !errors.Is(err, errFakeClosed) {
		t.Fatalf("Read error = %v, want %v", err, errFakeClosed)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("replacement port was not published")
	}

	if got := u.loadConn(); got != newPort {
		t.Fatalf("loadConn() = %T, want replacement port", got)
	}
}
