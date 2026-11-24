package connWrap

import (
	"errors"
	"io"
	"strconv"
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

// Reopener is implemented by transports that can drop the underlying link and
// establish it again (a serial port, a TCP connection).
type Reopener interface {
	// Reopen closes and reopens the link in the background and returns at once.
	// reason is only used for logging.
	Reopen(reason string)
}

const (
	DefaultBusQuietTime = 500 * time.Millisecond

	// DefaultSilentRequestLimit is how many consecutive requests may go completely
	// unanswered (not a single byte received) before the bus reopens its transport.
	DefaultSilentRequestLimit = 5

	// maxSilentRequestLimit caps the back-off applied while the far end stays silent.
	maxSilentRequestLimit = 80

	// ReopenPause is how long a transport waits between closing its link and
	// opening it again.
	//
	// Closing a serial port drops DTR and opening it raises DTR again; boards with
	// an auto-reset circuit (Arduino) are reset by that pulse. Done back to back the
	// pulse lasts a few milliseconds, which the reset capacitor may not turn into a
	// clean reset, so keep the two steps apart.
	ReopenPause = time.Second
)

// Bus serializes request/response exchanges over a shared transport.
//
// It also notices a transport that has gone deaf: writes succeed, nothing
// reports an error, and yet no byte ever comes back. That is all this side sees
// when a USB-serial bridge stops receiving or the microcontroller behind it
// hangs, and a transport reconnects by itself only on I/O errors, never on
// timeouts, so without help the bus would keep timing out until the process is
// restarted. After DefaultSilentRequestLimit consecutive requests without a
// single received byte the bus therefore asks the transport to reopen.
//
// Any received byte counts as a sign of life, so one dead device on a shared
// bus does not trigger a reopen while its neighbours still answer, and reads
// that time out without a preceding request never do.
type Bus struct {
	ConnCommon
	mu        sync.Mutex
	quietTime time.Duration

	// Guarded by mu, like every exchange on the bus.
	silentRequests int // requests written since the last received byte
	silentLimit    int // silentRequests at which the transport is reopened
}

func NewBus(conn ConnCommon) *Bus {
	return NewBusWithQuietTime(conn, DefaultBusQuietTime)
}

// NewBusWithQuietTime exists so tests can shorten the bus settling delay.
func NewBusWithQuietTime(conn ConnCommon, quietTime time.Duration) *Bus {
	return &Bus{
		ConnCommon:  conn,
		quietTime:   quietTime,
		silentLimit: DefaultSilentRequestLimit,
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
	n, err = b.ConnCommon.Write(input)
	if err == nil {
		b.silentRequests++
	}
	return n, err
}

// Read reads reply bytes for the request written last.
// The caller must already hold the bus lock.
func (b *Bus) Read(p []byte) (n int, err error) {
	n, err = b.ConnCommon.Read(p)
	if n > 0 {
		b.silentRequests = 0
		b.silentLimit = DefaultSilentRequestLimit
		return n, err
	}
	if errors.Is(err, ErrTimeout) && b.silentRequests >= b.silentLimit {
		b.reopenSilentTransport()
	}
	return n, err
}

func (b *Bus) reopenSilentTransport() {
	reopener, ok := b.ConnCommon.(Reopener)
	if !ok {
		return
	}
	reason := "no reply to the last " + strconv.Itoa(b.silentRequests) + " requests"
	b.silentRequests = 0
	// Back off while the far end stays silent: the port of a sensor that is
	// unplugged or dead should not be reopened every few requests forever.
	if b.silentLimit < maxSilentRequestLimit {
		b.silentLimit *= 2
	}
	reopener.Reopen(reason)
}

func (b *Bus) Lock() {
	b.mu.Lock()
}

func (b *Bus) Unlock() {
	b.mu.Unlock()
}
