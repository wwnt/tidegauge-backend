package sdi12

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

	"tide/tide_client/connWrap"
)

type stubConnCommon struct {
	readResults []stubReadResult
	readIndex   int
	readOffset  int
	writes      [][]byte
	writeSignal chan struct{}
	resetCalls  int
}

type stubReadResult struct {
	data []byte
	err  error
}

func (s *stubConnCommon) Read(p []byte) (int, error) {
	for s.readIndex < len(s.readResults) {
		res := s.readResults[s.readIndex]
		if s.readOffset < len(res.data) {
			n := copy(p, res.data[s.readOffset:])
			s.readOffset += n
			if s.readOffset == len(res.data) {
				s.readIndex++
				s.readOffset = 0
				return n, res.err
			}
			return n, nil
		}
		s.readIndex++
		s.readOffset = 0
		if res.err != nil {
			return 0, res.err
		}
	}
	return 0, io.EOF
}

func (s *stubConnCommon) Write(p []byte) (int, error) {
	s.writes = append(s.writes, append([]byte(nil), p...))
	if s.writeSignal != nil {
		select {
		case s.writeSignal <- struct{}{}:
		default:
		}
	}
	return len(p), nil
}

func (s *stubConnCommon) ResetInputBuffer() error {
	s.resetCalls++
	return nil
}

func waitForWrite(t *testing.T, rawConn *stubConnCommon) {
	t.Helper()

	select {
	case <-rawConn.writeSignal:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for request write")
	}
}

func waitForBusUnlock(t *testing.T, bus *connWrap.Bus) time.Duration {
	t.Helper()

	locked := make(chan time.Duration, 1)
	start := time.Now()
	go func() {
		bus.Lock()
		locked <- time.Since(start)
		bus.Unlock()
	}()

	select {
	case d := <-locked:
		return d
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for bus unlock")
		return 0
	}
}

func TestConcurrentMeasurementFramesArduinoCommand(t *testing.T) {
	t.Parallel()

	rawConn := &stubConnCommon{
		readResults: []stubReadResult{{data: []byte("0")}},
	}
	session := NewSession(connWrap.NewBusWithQuietTime(rawConn, 0), ModeArduino)

	if err := session.ConcurrentMeasurement("0", 7, "0", 0); err != nil {
		t.Fatalf("ConcurrentMeasurement returned error: %v", err)
	}

	want := []byte{'0', 'C', '!', 7, arduinoCommandEnd}
	if len(rawConn.writes) != 1 || !bytes.Equal(rawConn.writes[0], want) {
		t.Fatalf("writes = %v, want %v", rawConn.writes, want)
	}
	if rawConn.resetCalls != 1 {
		t.Fatalf("ResetInputBuffer called %d times, want 1", rawConn.resetCalls)
	}
}

func TestGetDataParsesMultipleArduinoResponses(t *testing.T) {
	t.Parallel()

	rawConn := &stubConnCommon{
		readResults: []stubReadResult{
			{data: []byte("0+1.23+4.56\r\n")},
			{data: []byte("0-7.89\r\n")},
		},
	}
	session := NewSession(connWrap.NewBusWithQuietTime(rawConn, 0), ModeArduino)

	values, err := session.GetData("0", 3, 3)
	if err != nil {
		t.Fatalf("GetData returned error: %v", err)
	}

	wantValues := []float64{1.23, 4.56, -7.89}
	if len(values) != len(wantValues) {
		t.Fatalf("len(values) = %d, want %d", len(values), len(wantValues))
	}
	for i, want := range wantValues {
		if values[i] == nil || *values[i] != want {
			t.Fatalf("values[%d] = %v, want %v", i, values[i], want)
		}
	}

	wantWrites := [][]byte{
		{'0', 'D', '0', '!', 3, arduinoCommandEnd},
		{'0', 'D', '1', '!', 3, arduinoCommandEnd},
	}
	if len(rawConn.writes) != len(wantWrites) {
		t.Fatalf("len(writes) = %d, want %d", len(rawConn.writes), len(wantWrites))
	}
	for i, want := range wantWrites {
		if !bytes.Equal(rawConn.writes[i], want) {
			t.Fatalf("writes[%d] = %v, want %v", i, rawConn.writes[i], want)
		}
	}
	if rawConn.resetCalls != len(wantWrites) {
		t.Fatalf("ResetInputBuffer called %d times, want %d", rawConn.resetCalls, len(wantWrites))
	}
}

func TestConcurrentMeasurementHoldsBusAfterParseError(t *testing.T) {
	rawConn := &stubConnCommon{
		readResults: []stubReadResult{{data: []byte("1")}},
		writeSignal: make(chan struct{}, 1),
	}
	session := NewSession(connWrap.NewBusWithQuietTime(rawConn, 0), ModeNative)

	errCh := make(chan error, 1)
	go func() {
		errCh <- session.ConcurrentMeasurement("0", 0, "0", 0)
	}()

	waitForWrite(t, rawConn)
	lockedAfter := waitForBusUnlock(t, session.bus)
	err := <-errCh
	if err == nil {
		t.Fatal("ConcurrentMeasurement returned nil error, want parse error")
	}
	if errors.Is(err, connWrap.ErrTimeout) {
		t.Fatalf("ConcurrentMeasurement returned timeout error, want non-timeout parse error: %v", err)
	}
	if lockedAfter < 800*time.Millisecond {
		t.Fatalf("bus unlocked after %v, want at least 800ms settle time", lockedAfter)
	}
}

func TestGetDataHoldsBusAfterParseError(t *testing.T) {
	rawConn := &stubConnCommon{
		readResults: []stubReadResult{{data: []byte("0x\r\n")}},
		writeSignal: make(chan struct{}, 1),
	}
	session := NewSession(connWrap.NewBusWithQuietTime(rawConn, 0), ModeNative)

	errCh := make(chan error, 1)
	go func() {
		_, err := session.GetData("0", 0, 1)
		errCh <- err
	}()

	waitForWrite(t, rawConn)
	lockedAfter := waitForBusUnlock(t, session.bus)
	err := <-errCh
	if err == nil {
		t.Fatal("GetData returned nil error, want parse error")
	}
	if errors.Is(err, connWrap.ErrTimeout) {
		t.Fatalf("GetData returned timeout error, want non-timeout parse error: %v", err)
	}
	if lockedAfter < 800*time.Millisecond {
		t.Fatalf("bus unlocked after %v, want at least 800ms settle time", lockedAfter)
	}
}

// reopenableConn 是支持重开的 stubConnCommon，记录被要求重开的次数。
type reopenableConn struct {
	stubConnCommon
	reopens int
}

func (c *reopenableConn) Reopen(string) { c.reopens++ }

// 现场事故的复现：Arduino 不再回任何字节，每次 C! 都只是读超时，串口本身不报错。
// 连续 DefaultSilentRequestLimit 次之后必须要求传输层重开端口，否则会一直超时到进程重启。
func TestConcurrentMeasurementReopensTransportThatStoppedAnswering(t *testing.T) {
	rawConn := &reopenableConn{}
	for range connWrap.DefaultSilentRequestLimit {
		rawConn.readResults = append(rawConn.readResults, stubReadResult{err: connWrap.ErrTimeout})
	}
	session := NewSession(connWrap.NewBusWithQuietTime(rawConn, 0), ModeArduino)

	for attempt := 1; attempt <= connWrap.DefaultSilentRequestLimit; attempt++ {
		err := session.ConcurrentMeasurement("1", 4, "100505\r\n", 0)
		if !errors.Is(err, connWrap.ErrTimeout) {
			t.Fatalf("attempt %d: error = %v, want a timeout", attempt, err)
		}

		wantReopens := 0
		if attempt == connWrap.DefaultSilentRequestLimit {
			wantReopens = 1
		}
		if rawConn.reopens != wantReopens {
			t.Fatalf("attempt %d: transport reopened %d times, want %d", attempt, rawConn.reopens, wantReopens)
		}
	}
}
