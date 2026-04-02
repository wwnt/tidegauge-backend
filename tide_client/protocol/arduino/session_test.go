package arduino

import (
	"bytes"
	"io"
	"testing"

	"tide/tide_client/connWrap"
)

type stubConnCommon struct {
	readResults []stubReadResult
	readIndex   int
	readOffset  int
	writes      [][]byte
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
	return len(p), nil
}

func (s *stubConnCommon) ResetInputBuffer() error {
	s.resetCalls++
	return nil
}

func TestAnalogReadWritesArduinoFrameAndParsesValue(t *testing.T) {
	t.Parallel()

	rawConn := &stubConnCommon{
		readResults: []stubReadResult{{data: []byte("512\r\n")}},
	}
	session := NewSession(connWrap.NewBusWithQuietTime(rawConn, 0))

	got, err := session.AnalogRead(3)
	if err != nil {
		t.Fatalf("AnalogRead returned error: %v", err)
	}
	if got != 512 {
		t.Fatalf("AnalogRead = %d, want 512", got)
	}

	wantWrite := []byte{3, commandEnd}
	if len(rawConn.writes) != 1 || !bytes.Equal(rawConn.writes[0], wantWrite) {
		t.Fatalf("writes = %v, want %v", rawConn.writes, wantWrite)
	}
	if rawConn.resetCalls != 1 {
		t.Fatalf("ResetInputBuffer called %d times, want 1", rawConn.resetCalls)
	}
}
