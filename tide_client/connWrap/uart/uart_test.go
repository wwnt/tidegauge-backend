package uart

import (
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"go.bug.st/serial"

	"tide/tide_client/connWrap"
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

// waitForPort 等到 u 发布了 want 这个端口；重开是在后台 goroutine 里完成的。
func waitForPort(t *testing.T, u *Uart, want serial.Port) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for u.loadConn() != want {
		if time.Now().After(deadline) {
			t.Fatal("replacement port was not published")
		}
		time.Sleep(time.Millisecond)
	}
}

// Reopen 必须先关闭旧端口，隔开 reopenPause 之后再打开新端口：
// 这段间隔就是 DTR 复位脉冲的宽度。
func TestReopenClosesPortThenOpensANewOne(t *testing.T) {
	const pause = 60 * time.Millisecond
	oldPort := newFakePort(errFakeClosed)
	newPort := newFakePort(nil)

	type openCall struct {
		oldPortClosed bool
		sinceReopen   time.Duration
	}
	opened := make(chan openCall, 1)
	var start time.Time
	u := &Uart{
		portName:    "COM1",
		conn:        oldPort,
		reopenPause: pause,
		openPort: func(string, *serial.Mode) (serial.Port, error) {
			call := openCall{sinceReopen: time.Since(start)}
			select {
			case <-oldPort.closed:
				call.oldPortClosed = true
			default:
			}
			opened <- call
			return newPort, nil
		},
	}

	start = time.Now()
	u.Reopen("test")

	select {
	case call := <-opened:
		if !call.oldPortClosed {
			t.Fatal("the new port was opened before the old one was closed")
		}
		if call.sinceReopen < pause {
			t.Fatalf("the new port was opened %v after Reopen, want at least %v", call.sinceReopen, pause)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Reopen did not open a new port")
	}
	waitForPort(t, u, newPort)

	if _, err := u.Write([]byte{0x01}); err != nil {
		t.Fatalf("Write after Reopen returned error: %v", err)
	}
	if oldWrites, newWrites := oldPort.writes.Load(), newPort.writes.Load(); oldWrites != 0 || newWrites != 1 {
		t.Fatalf("writes: old port %d, new port %d, want 0 and 1", oldWrites, newWrites)
	}
}

// 端到端：串口不报任何错误，但对端一个字节都不回。总线上连续
// DefaultSilentRequestLimit 个请求没有回音后，串口被重开，之后的请求走新端口。
func TestBusReopensSilentUart(t *testing.T) {
	oldPort := newFakePort(errFakeClosed) // 打开状态下 Read 返回 (0, nil)，即读超时
	newPort := newFakePort(nil)
	var opens atomic.Int32
	u := &Uart{
		portName: "COM1",
		conn:     oldPort,
		openPort: func(string, *serial.Mode) (serial.Port, error) {
			opens.Add(1)
			return newPort, nil
		},
	}
	bus := connWrap.NewBusWithQuietTime(u, 0)

	request := func() error {
		bus.Lock()
		defer bus.Unlock()
		if _, err := bus.Write([]byte{0x01}); err != nil {
			return err
		}
		var buf [1]byte
		_, err := bus.Read(buf[:])
		return err
	}

	for i := 1; i < connWrap.DefaultSilentRequestLimit; i++ {
		if err := request(); !errors.Is(err, connWrap.ErrTimeout) {
			t.Fatalf("request %d error = %v, want a timeout", i, err)
		}
	}
	if opens.Load() != 0 || u.inReconnect.Load() {
		t.Fatalf("port was reopened after only %d silent requests", connWrap.DefaultSilentRequestLimit-1)
	}

	if err := request(); !errors.Is(err, connWrap.ErrTimeout) {
		t.Fatalf("request %d error = %v, want a timeout", connWrap.DefaultSilentRequestLimit, err)
	}
	waitForPort(t, u, newPort)

	select {
	case <-oldPort.closed:
	default:
		t.Fatal("the silent port was not closed")
	}
	if got := opens.Load(); got != 1 {
		t.Fatalf("port opened %d times, want 1", got)
	}

	if err := request(); !errors.Is(err, connWrap.ErrTimeout) {
		t.Fatalf("request after reopen error = %v, want a timeout", err)
	}
	if oldWrites, newWrites := oldPort.writes.Load(), newPort.writes.Load(); oldWrites != int32(connWrap.DefaultSilentRequestLimit) || newWrites != 1 {
		t.Fatalf("writes: old port %d, new port %d, want %d and 1", oldWrites, newWrites, connWrap.DefaultSilentRequestLimit)
	}
}
