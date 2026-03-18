package pbstream

import (
	"io"
	"net"
	"strconv"
	"sync"
	"testing"

	syncpb "tide/pkg/pb/syncproto"

	"github.com/stretchr/testify/require"
)

func TestTypedDelimitedStreamStationFrame(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer func() { _ = clientConn.Close() }()
	defer func() { _ = serverConn.Close() }()

	clientStream := NewTypedDelimitedStream[*syncpb.StationMessage](
		clientConn,
		32<<20,
		func() *syncpb.StationMessage { return &syncpb.StationMessage{} },
	)
	serverStream := NewTypedDelimitedStream[*syncpb.StationMessage](
		serverConn,
		32<<20,
		func() *syncpb.StationMessage { return &syncpb.StationMessage{} },
	)

	want := &syncpb.StationMessage{
		Body: &syncpb.StationMessage_Error{
			Error: &syncpb.ErrorFrame{Message: "hello-station"},
		},
	}
	go func() { _ = clientStream.Send(want) }()

	got, err := serverStream.Recv()
	require.NoError(t, err)
	errBody, ok := got.Body.(*syncpb.StationMessage_Error)
	require.True(t, ok)
	require.Equal(t, "hello-station", errBody.Error.Message)
}

func TestTypedDelimitedStreamRelayFrame(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer func() { _ = clientConn.Close() }()
	defer func() { _ = serverConn.Close() }()

	clientStream := NewTypedDelimitedStream[*syncpb.RelayMessage](
		clientConn,
		32<<20,
		func() *syncpb.RelayMessage { return &syncpb.RelayMessage{} },
	)
	serverStream := NewTypedDelimitedStream[*syncpb.RelayMessage](
		serverConn,
		32<<20,
		func() *syncpb.RelayMessage { return &syncpb.RelayMessage{} },
	)

	want := &syncpb.RelayMessage{
		Body: &syncpb.RelayMessage_DownstreamHello{
			DownstreamHello: &syncpb.RelayDownstreamHello{ProtocolVersion: "v2"},
		},
	}
	go func() { _ = clientStream.Send(want) }()

	got, err := serverStream.Recv()
	require.NoError(t, err)
	hello, ok := got.Body.(*syncpb.RelayMessage_DownstreamHello)
	require.True(t, ok)
	require.Equal(t, "v2", hello.DownstreamHello.ProtocolVersion)
}

func TestTypedDelimitedStreamConcurrentSend(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer func() { _ = clientConn.Close() }()
	defer func() { _ = serverConn.Close() }()

	clientStream := NewTypedDelimitedStream[*syncpb.StationMessage](
		clientConn,
		32<<20,
		func() *syncpb.StationMessage { return &syncpb.StationMessage{} },
	)
	serverStream := NewTypedDelimitedStream[*syncpb.StationMessage](
		serverConn,
		32<<20,
		func() *syncpb.StationMessage { return &syncpb.StationMessage{} },
	)

	const count = 64

	recvDone := make(chan map[string]struct{}, 1)
	recvErr := make(chan error, 1)
	go func() {
		got := make(map[string]struct{}, count)
		for i := 0; i < count; i++ {
			frame, err := serverStream.Recv()
			if err != nil {
				recvErr <- err
				return
			}
			errBody, ok := frame.Body.(*syncpb.StationMessage_Error)
			if !ok || errBody.Error == nil {
				recvErr <- io.ErrUnexpectedEOF
				return
			}
			got[errBody.Error.Message] = struct{}{}
		}
		recvDone <- got
	}()

	var wg sync.WaitGroup
	wg.Add(count)
	for i := 0; i < count; i++ {
		i := i
		go func() {
			defer wg.Done()
			_ = clientStream.Send(&syncpb.StationMessage{
				Body: &syncpb.StationMessage_Error{
					Error: &syncpb.ErrorFrame{Message: "msg-" + strconv.Itoa(i)},
				},
			})
		}()
	}
	wg.Wait()

	select {
	case err := <-recvErr:
		require.NoError(t, err)
	case got := <-recvDone:
		require.Len(t, got, count)
	}
}
