package uart

import (
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"go.bug.st/serial"
)

var errFakeClosed = errors.New("fake port closed")

// 端口被重连流程关闭后，Write/ResetInputBuffer 不得再调用到旧端口对象：真实的串口库
// 在这两个方法里不检查关闭状态，会对一个可能已被别的总线复用的 fd 号做系统调用。
func TestWriteAndResetAfterCloseDoNotTouchClosedPort(t *testing.T) {
	oldPort := newFakePort(errFakeClosed)
	u := &Uart{portName: "COM1", conn: oldPort}

	u.closeCurrent()

	if n, err := u.Write([]byte{0x01}); !errors.Is(err, os.ErrClosed) || n != 0 {
		t.Fatalf("Write = (%d, %v), want (0, os.ErrClosed)", n, err)
	}
	if err := u.ResetInputBuffer(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("ResetInputBuffer error = %v, want os.ErrClosed", err)
	}
	if w, r := oldPort.writes.Load(), oldPort.resets.Load(); w != 0 || r != 0 {
		t.Fatalf("closed port was used: writes=%d resets=%d, want 0 0", w, r)
	}
	if u.inReconnect.Load() {
		t.Fatal("an error from the closed-port guard must not start another reconnect")
	}

	// 新端口就绪后恢复正常。
	newPort := newFakePort(nil)
	u.storeConn(newPort)
	if _, err := u.Write([]byte{0x01}); err != nil {
		t.Fatalf("Write after reconnect returned error: %v", err)
	}
	if err := u.ResetInputBuffer(); err != nil {
		t.Fatalf("ResetInputBuffer after reconnect returned error: %v", err)
	}
	if w, r := newPort.writes.Load(), newPort.resets.Load(); w != 1 || r != 1 {
		t.Fatalf("replacement port: writes=%d resets=%d, want 1 1", w, r)
	}
}

type fakePort struct {
	closed  chan struct{}
	readErr error
	writes  atomic.Int32
	resets  atomic.Int32
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

func (p *fakePort) Write(b []byte) (int, error) {
	p.writes.Add(1)
	return len(b), nil
}

func (p *fakePort) Drain() error { return nil }

func (p *fakePort) ResetInputBuffer() error {
	p.resets.Add(1)
	return nil
}

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
