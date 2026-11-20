package pbstream

import (
	"bufio"
	"errors"
	"io"
	"sync"
	"time"

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

// ErrDeadlineNotSupported 表示底层连接不支持读超时。
var ErrDeadlineNotSupported = errors.New("pbstream: underlying connection does not support read deadlines")

// SetReadDeadline 给后续以及正在阻塞中的 Recv 设置截止时间；零值表示取消。
//
// 需要它的原因：Close 不一定能打断阻塞中的 Recv——底层是 yamux 流时，Close 只是
// 半关闭（停止本端写），本端的读会一直等到对端关闭。要从另一个 goroutine 中止
// Recv，应当设置一个已经过去的截止时间。
func (s *DelimitedStream) SetReadDeadline(t time.Time) error {
	if d, ok := s.conn.(interface{ SetReadDeadline(time.Time) error }); ok {
		return d.SetReadDeadline(t)
	}
	return ErrDeadlineNotSupported
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

// SetReadDeadline 见 DelimitedStream.SetReadDeadline。
func (s *TypedDelimitedStream[T]) SetReadDeadline(t time.Time) error {
	return s.raw.SetReadDeadline(t)
}
