package textline

import (
	"io"
	"testing"

	"tide/tide_client/connWrap"
)

type stubConnCommon struct {
	readResults []stubReadResult
	readIndex   int
	readOffset  int
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
	return len(p), nil
}

func (s *stubConnCommon) ResetInputBuffer() error {
	return nil
}

func TestCustomCommandTreatsTimeoutAfterDataAsSuccess(t *testing.T) {
	t.Parallel()

	session := NewSession(connWrap.NewBusWithQuietTime(&stubConnCommon{
		readResults: []stubReadResult{
			{data: []byte("STATUS")},
			{err: connWrap.ErrTimeout},
		},
	}, 0))
	session.customCommandWait = 0

	received, err := session.CustomCommand([]byte("cmd"))
	if err != nil {
		t.Fatalf("CustomCommand returned error: %v", err)
	}
	if string(received) != "STATUS" {
		t.Fatalf("CustomCommand received %q, want %q", string(received), "STATUS")
	}
}
