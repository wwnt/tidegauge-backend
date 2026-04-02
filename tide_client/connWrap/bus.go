package connWrap

import (
	"io"
	"sync"
	"time"
)

type ConnCommon interface {
	io.ReadWriter
	ResetInputBuffer() (err error)
}

type SerialConfigProvider interface {
	SerialBaudRate() int
}

const DefaultBusQuietTime = 500 * time.Millisecond

// Bus serializes request/response exchanges over a shared transport.
type Bus struct {
	ConnCommon
	mu        sync.Mutex
	quietTime time.Duration
}

func NewBus(conn ConnCommon) *Bus {
	return NewBusWithQuietTime(conn, DefaultBusQuietTime)
}

// NewBusWithQuietTime exists so tests can shorten the bus settling delay.
func NewBusWithQuietTime(conn ConnCommon, quietTime time.Duration) *Bus {
	return &Bus{
		ConnCommon: conn,
		quietTime:  quietTime,
	}
}

func (b *Bus) SerialBaudRate() int {
	if provider, ok := b.ConnCommon.(SerialConfigProvider); ok {
		return provider.SerialBaudRate()
	}
	return 0
}

// Write sends one request on the shared bus.
// The caller must already hold the bus lock.
// It preserves the legacy order: wait for the bus to go quiet, clear any
// trailing bytes from the previous exchange, then write the new request bytes.
func (b *Bus) Write(input []byte) (n int, err error) {
	time.Sleep(b.quietTime)
	if err = b.ResetInputBuffer(); err != nil {
		return 0, err
	}
	return b.ConnCommon.Write(input)
}

func (b *Bus) Lock() {
	b.mu.Lock()
}

func (b *Bus) Unlock() {
	b.mu.Unlock()
}
