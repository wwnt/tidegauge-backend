package syncv2

import (
	"io"

	syncpb "tide/pkg/pb/syncproto"
	"tide/pkg/pbstream"
)

// StationMessageStream is the concrete delimited stream for StationMessage.
// Send is safe for concurrent calls because pbstream.DelimitedStream serializes writes.
type StationMessageStream = *pbstream.TypedDelimitedStream[*syncpb.StationMessage]

// RelayMessageStream is the varint-delimited protobuf stream for relay (server-to-server) RelayMessage messages.
// We intentionally avoid naming it "Sync*" to reduce ambiguity with station sync.
type RelayMessageStream = *pbstream.TypedDelimitedStream[*syncpb.RelayMessage]

func NewStationMessageStream(conn io.ReadWriteCloser, maxFrameBytes int64) StationMessageStream {
	return pbstream.NewTypedDelimitedStream[*syncpb.StationMessage](
		conn,
		maxFrameBytes,
		func() *syncpb.StationMessage { return &syncpb.StationMessage{} },
	)
}

func NewRelayMessageStream(conn io.ReadWriteCloser, maxFrameBytes int64) RelayMessageStream {
	return pbstream.NewTypedDelimitedStream[*syncpb.RelayMessage](
		conn,
		maxFrameBytes,
		func() *syncpb.RelayMessage { return &syncpb.RelayMessage{} },
	)
}
