package connWrap

import (
	"errors"
	"io"
	"testing"
	"time"
)

type writeOrderConn struct {
	resetAt time.Time
	writeAt time.Time
}

func (c *writeOrderConn) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (c *writeOrderConn) Write(p []byte) (int, error) {
	c.writeAt = time.Now()
	return len(p), nil
}

func (c *writeOrderConn) ResetInputBuffer() error {
	c.resetAt = time.Now()
	return nil
}

func TestWriteWaitsBeforeResettingInput(t *testing.T) {
	t.Parallel()

	const quietTime = 50 * time.Millisecond
	const tolerance = 10 * time.Millisecond

	rawConn := &writeOrderConn{}
	bus := NewBusWithQuietTime(rawConn, quietTime)

	start := time.Now()
	if _, err := bus.Write([]byte("cmd")); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	if rawConn.resetAt.IsZero() {
		t.Fatal("ResetInputBuffer was not called")
	}
	if rawConn.writeAt.IsZero() {
		t.Fatal("Write was not called")
	}
	if delay := rawConn.resetAt.Sub(start); delay < quietTime-tolerance {
		t.Fatalf("ResetInputBuffer called after %v, want at least %v", delay, quietTime)
	}
	if rawConn.writeAt.Before(rawConn.resetAt) {
		t.Fatalf("Write happened before ResetInputBuffer: writeAt=%v resetAt=%v", rawConn.writeAt, rawConn.resetAt)
	}
}

// scriptedConn 按脚本应答请求：replies 里的每一项对应一次请求，非空字符串是该请求的应答，
// 空字符串表示对端不回话（读超时）。它同时实现 Reopener 并记录每次重开的原因。
type scriptedConn struct {
	replies  []string
	pending  string
	writeErr error
	reopens  []string
}

func (c *scriptedConn) Read(p []byte) (int, error) {
	if c.pending == "" {
		return 0, ErrTimeout
	}
	n := copy(p, c.pending)
	c.pending = c.pending[n:]
	return n, nil
}

func (c *scriptedConn) Write(p []byte) (int, error) {
	if c.writeErr != nil {
		return 0, c.writeErr
	}
	if len(c.replies) > 0 {
		c.pending, c.replies = c.replies[0], c.replies[1:]
	}
	return len(p), nil
}

func (c *scriptedConn) ResetInputBuffer() error {
	c.pending = ""
	return nil
}

func (c *scriptedConn) Reopen(reason string) {
	c.reopens = append(c.reopens, reason)
}

// exchange 模拟各协议 session 的一次请求/应答：加锁、发请求、读应答直到出错。
func exchange(bus *Bus) (reply string, err error) {
	bus.Lock()
	defer bus.Unlock()

	if _, err = bus.Write([]byte("request")); err != nil {
		return "", err
	}
	buf := make([]byte, 16)
	for {
		n, readErr := bus.Read(buf)
		reply += string(buf[:n])
		if readErr != nil {
			if errors.Is(readErr, ErrTimeout) && reply != "" {
				return reply, nil
			}
			return reply, readErr
		}
	}
}

func silent(n int) []string { return make([]string, n) }

func TestBusReopensTransportAfterConsecutiveSilentRequests(t *testing.T) {
	conn := &scriptedConn{}
	bus := NewBusWithQuietTime(conn, 0)

	run := func(replies ...string) {
		t.Helper()
		conn.replies = replies
		for range replies {
			_, _ = exchange(bus)
		}
	}

	run(silent(DefaultSilentRequestLimit - 1)...)
	if len(conn.reopens) != 0 {
		t.Fatalf("reopened after %d silent requests, want none yet: %v", DefaultSilentRequestLimit-1, conn.reopens)
	}
	run("")
	if len(conn.reopens) != 1 || conn.reopens[0] != "no reply to the last 5 requests" {
		t.Fatalf("reopens after %d silent requests = %q, want exactly one", DefaultSilentRequestLimit, conn.reopens)
	}

	// 重开之后对端仍然不回话：下一次重开要等两倍的请求数，不能每 5 个请求就重开一次。
	run(silent(2*DefaultSilentRequestLimit - 1)...)
	if len(conn.reopens) != 1 {
		t.Fatalf("reopened again before the backed-off limit: %q", conn.reopens)
	}
	run("")
	if len(conn.reopens) != 2 || conn.reopens[1] != "no reply to the last 10 requests" {
		t.Fatalf("reopens = %q, want a second one after 10 more silent requests", conn.reopens)
	}

	// 只要收到过应答，退避就复位，之后又按最初的阈值计数。
	run("reply")
	run(silent(DefaultSilentRequestLimit)...)
	if len(conn.reopens) != 3 || conn.reopens[2] != "no reply to the last 5 requests" {
		t.Fatalf("reopens = %q, want a third one 5 silent requests after a reply", conn.reopens)
	}
}

func TestBusSilentLimitBackoffIsCapped(t *testing.T) {
	conn := &scriptedConn{}
	bus := NewBusWithQuietTime(conn, 0)

	// 5+10+20+40+80 个请求触发前 5 次重开，之后每 80 个请求一次。
	const requests = 5 + 10 + 20 + 40 + 80 + 3*maxSilentRequestLimit
	for range requests {
		_, _ = exchange(bus)
	}
	if got, want := len(conn.reopens), 5+3; got != want {
		t.Fatalf("reopens after %d silent requests = %d, want %d", requests, got, want)
	}
}

// 共享总线上一台设备坏了、其余设备正常应答时，不能去重开整条总线。
func TestBusKeepsTransportWhileAnyDeviceAnswers(t *testing.T) {
	conn := &scriptedConn{}
	bus := NewBusWithQuietTime(conn, 0)

	for range 20 {
		conn.replies = append(silent(DefaultSilentRequestLimit-1), "reply")
		for range DefaultSilentRequestLimit {
			_, _ = exchange(bus)
		}
	}
	if len(conn.reopens) != 0 {
		t.Fatalf("reopened although every fifth request was answered: %q", conn.reopens)
	}
}

// 没有发过请求的读超时（例如等设备主动上报）不是"对端不回话"。
func TestBusIgnoresTimeoutsWithoutRequests(t *testing.T) {
	conn := &scriptedConn{}
	bus := NewBusWithQuietTime(conn, 0)

	buf := make([]byte, 16)
	for range 10 * maxSilentRequestLimit {
		if _, err := bus.Read(buf); !errors.Is(err, ErrTimeout) {
			t.Fatalf("Read error = %v, want ErrTimeout", err)
		}
	}
	if len(conn.reopens) != 0 {
		t.Fatalf("reopened without a single request: %q", conn.reopens)
	}
}

// 写失败的请求没有发出去，不计入"没有回音"；写错误由传输层自己的重连处理。
func TestBusDoesNotCountFailedWrites(t *testing.T) {
	conn := &scriptedConn{writeErr: io.ErrClosedPipe}
	bus := NewBusWithQuietTime(conn, 0)

	for range 10 * DefaultSilentRequestLimit {
		if _, err := exchange(bus); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("exchange error = %v, want the write error", err)
		}
	}
	// 写恢复后要重新数满阈值才重开。
	conn.writeErr = nil
	for range DefaultSilentRequestLimit - 1 {
		_, _ = exchange(bus)
	}
	if len(conn.reopens) != 0 {
		t.Fatalf("failed writes were counted as silent requests: %q", conn.reopens)
	}
}

// 不支持重开的传输（测试替身等）照常返回超时，不受影响。
func TestBusWithoutReopenerOnlyReportsTimeouts(t *testing.T) {
	bus := NewBusWithQuietTime(timeoutConn{}, 0)

	for range 3 * DefaultSilentRequestLimit {
		if _, err := exchange(bus); !errors.Is(err, ErrTimeout) {
			t.Fatalf("exchange error = %v, want ErrTimeout", err)
		}
	}
}

type timeoutConn struct{}

func (timeoutConn) Read([]byte) (int, error)    { return 0, ErrTimeout }
func (timeoutConn) Write(p []byte) (int, error) { return len(p), nil }
func (timeoutConn) ResetInputBuffer() error     { return nil }
