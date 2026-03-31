package connWrap

import (
	"io"
	"testing"
)

type stubConnCommon struct {
	readResults []stubReadResult
	readIndex   int
}

type stubReadResult struct {
	data []byte
	err  error
}

func (s *stubConnCommon) Read(p []byte) (int, error) {
	if s.readIndex >= len(s.readResults) {
		return 0, io.EOF
	}
	res := s.readResults[s.readIndex]
	s.readIndex++
	copy(p, res.data)
	return len(res.data), res.err
}

func (s *stubConnCommon) Write(p []byte) (int, error) {
	return len(p), nil
}

func (s *stubConnCommon) ResetInputBuffer() error {
	return nil
}

func TestCustomCommandTreatsTimeoutAfterDataAsSuccess(t *testing.T) {
	t.Parallel()

	conn := NewConnUtil(&stubConnCommon{
		readResults: []stubReadResult{
			{data: []byte("STATUS")},
			{err: ErrTimeout},
		},
	})

	received, err := conn.CustomCommand([]byte("cmd"))
	if err != nil {
		t.Fatalf("CustomCommand returned error: %v", err)
	}
	if string(received) != "STATUS" {
		t.Fatalf("CustomCommand received %q, want %q", string(received), "STATUS")
	}
}
