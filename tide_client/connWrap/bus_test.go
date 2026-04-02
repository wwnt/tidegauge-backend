package connWrap

import (
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
