package pbstream

import (
	"bufio"
	"io"
	"sync"

	"google.golang.org/protobuf/encoding/protodelim"
	"google.golang.org/protobuf/proto"
)

// DelimitedStream reads and writes protobuf messages in varint size-delimited format.
// It is safe for concurrent Send calls.
type DelimitedStream struct {
	conn io.ReadWriteCloser
	r    *bufio.Reader

	sendMu  sync.Mutex
	recvOpt protodelim.UnmarshalOptions
}

func NewDelimitedStream(conn io.ReadWriteCloser, maxFrameBytes int64) *DelimitedStream {
	return &DelimitedStream{
		conn: conn,
		r:    bufio.NewReader(conn),
		recvOpt: protodelim.UnmarshalOptions{
			MaxSize: maxFrameBytes,
		},
	}
}

func (s *DelimitedStream) Send(msg proto.Message) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	_, err := protodelim.MarshalTo(s.conn, msg)
	return err
}

func (s *DelimitedStream) Recv(msg proto.Message) error {
	return s.recvOpt.UnmarshalFrom(s.r, msg)
}

func (s *DelimitedStream) Close() error {
	return s.conn.Close()
}

// TypedDelimitedStream provides strongly-typed Send/Recv helpers on top of DelimitedStream.
type TypedDelimitedStream[T proto.Message] struct {
	raw    *DelimitedStream
	newMsg func() T
}

func NewTypedDelimitedStream[T proto.Message](conn io.ReadWriteCloser, maxFrameBytes int64, newMsg func() T) *TypedDelimitedStream[T] {
	return &TypedDelimitedStream[T]{
		raw:    NewDelimitedStream(conn, maxFrameBytes),
		newMsg: newMsg,
	}
}

func (s *TypedDelimitedStream[T]) Send(msg T) error {
	return s.raw.Send(msg)
}

func (s *TypedDelimitedStream[T]) Recv() (T, error) {
	var zero T
	msg := s.newMsg()
	if err := s.raw.Recv(msg); err != nil {
		return zero, err
	}
	return msg, nil
}

func (s *TypedDelimitedStream[T]) Close() error {
	return s.raw.Close()
}
