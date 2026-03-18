package syncv2

import (
	"context"
	"errors"
	"io"
	"net"
	"time"

	"tide/common"
	internalsyncv2 "tide/internal/syncv2"
	syncpb "tide/pkg/pb/syncproto"
	"tide/pkg/pubsub"

	"github.com/hashicorp/yamux"
)

const replayDataBatchSize = 128

type stationCommandSession interface {
	Accept() (net.Conn, error)
}

func (c *Client) runOnConn(ctx context.Context, conn net.Conn) error {
	muxCfg := yamux.DefaultConfig()
	muxCfg.EnableKeepAlive = false
	muxCfg.ConnectionWriteTimeout = 30 * time.Second
	session, err := yamux.Client(conn, muxCfg)
	if err != nil {
		return err
	}
	defer func() { _ = session.Close() }()

	go func() {
		<-ctx.Done()
		_ = session.Close()
	}()

	mainConn, err := session.Accept()
	if err != nil {
		return err
	}

	stream := internalsyncv2.NewStationMessageStream(mainConn, internalsyncv2.DefaultMaxFrameBytes)
	defer func() { _ = stream.Close() }()

	log := c.logger()
	commandErrCh := make(chan error, 1)
	go func() {
		commandErrCh <- c.serveCommandSubstreams(ctx, session)
	}()

	if err := c.sendMainFrame(ctx, stream, &syncpb.StationMessage{Body: &syncpb.StationMessage_ClientHello{
		ClientHello: &syncpb.ClientHello{
			StationIdentifier: c.cfg.StationIdentifier,
			ProtocolVersion:   internalsyncv2.ProtocolVersion,
		},
	}}); err != nil {
		return err
	}

	first, err := stream.Recv()
	if err != nil {
		return err
	}
	if _, ok := first.Body.(*syncpb.StationMessage_ServerHello); !ok {
		return errors.New("expected server_hello")
	}

	info := c.deps.StationInfoFn()
	if err := c.sendMainFrame(ctx, stream, &syncpb.StationMessage{
		Body: &syncpb.StationMessage_StationInfo{StationInfo: internalsyncv2.StationInfoToPB(info)},
	}); err != nil {
		return err
	}

	itemsLatest, latestStatusLogRowID, err := c.recvLatestFrames(stream)
	if err != nil {
		return err
	}

	subscriber := pubsub.NewSubscriber(10000, func() { _ = session.Close() })
	c.deps.IngestLock.Lock()
	err = c.sendReplayData(ctx, stream, info, itemsLatest)
	if err != nil {
		c.deps.IngestLock.Unlock()
		return err
	}
	err = c.sendReplayStatus(ctx, stream, latestStatusLogRowID)
	if err != nil {
		c.deps.IngestLock.Unlock()
		return err
	}
	c.deps.Subscribe(subscriber, nil)
	defer c.deps.Unsubscribe(subscriber)
	c.deps.IngestLock.Unlock()

	recvErrCh := make(chan error, 1)
	go func() {
		recvErrCh <- c.recvStationFrames(stream)
	}()

	log.Info("sync v2 connected, entering realtime", "addr", c.cfg.Addr, "station", c.cfg.StationIdentifier)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case err = <-recvErrCh:
			if err != nil && !errors.Is(err, io.EOF) {
				return err
			}
			return nil

		case err = <-commandErrCh:
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err != nil && !errors.Is(err, io.EOF) {
				return err
			}
			return nil

		case val, ok := <-subscriber.Ch:
			if !ok {
				return nil
			}
			if err = c.forwardRealtimeData(ctx, stream, val); err != nil {
				if errors.Is(err, context.Canceled) {
					return nil
				}
				log.Warn("forward realtime data failed", "error", err)
			}
		}
	}
}

func (c *Client) sendMainFrame(ctx context.Context, stream internalsyncv2.StationMessageStream, frame *syncpb.StationMessage) error {
	if frame == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	c.mainSendMu.Lock()
	defer c.mainSendMu.Unlock()
	return stream.Send(frame)
}

func (c *Client) recvLatestFrames(stream internalsyncv2.StationMessageStream) (map[string]int64, int64, error) {
	gotItems := false
	gotStatus := false
	itemsLatest := make(map[string]int64)
	var latestStatusLogRowID int64

	for !gotItems || !gotStatus {
		frame, err := stream.Recv()
		if err != nil {
			return nil, 0, err
		}
		switch body := frame.Body.(type) {
		case *syncpb.StationMessage_ItemsLatest:
			itemsLatest = body.ItemsLatest.LatestUnixMs
			gotItems = true
		case *syncpb.StationMessage_StatusLatest:
			latestStatusLogRowID = body.StatusLatest.LatestRowId
			gotStatus = true
		case *syncpb.StationMessage_Error:
			return nil, 0, errors.New(body.Error.Message)
		default:
			return nil, 0, unexpectedStationFrameError("waiting latest state")
		}
	}
	return itemsLatest, latestStatusLogRowID, nil
}

func (c *Client) recvStationFrames(stream internalsyncv2.StationMessageStream) error {
	for {
		frame, err := stream.Recv()
		if err != nil {
			return err
		}
		switch body := frame.Body.(type) {
		case *syncpb.StationMessage_Error:
			return errors.New(body.Error.Message)
		default:
			// ignore
		}
	}
}

func (c *Client) serveCommandSubstreams(ctx context.Context, session stationCommandSession) error {
	log := c.logger()

	for {
		conn, err := session.Accept()
		if err != nil {
			return err
		}

		go func(commandConn net.Conn) {
			if handleErr := c.handleCommandStream(ctx, commandConn); handleErr != nil && ctx.Err() == nil {
				log.Debug("station command stream failed", "error", handleErr)
			}
		}(conn)
	}
}

func (c *Client) handleCommandStream(ctx context.Context, conn io.ReadWriteCloser) error {
	stream := internalsyncv2.NewStationMessageStream(conn, internalsyncv2.DefaultMaxFrameBytes)
	defer func() { _ = stream.Close() }()

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	frame, err := stream.Recv()
	if err != nil {
		return err
	}

	switch body := frame.Body.(type) {
	case *syncpb.StationMessage_CameraSnapshotRequest:
		return stream.Send(buildSnapshotResponseFrame(body.CameraSnapshotRequest, c.deps.GetCamera, c.deps.Snapshot))
	case *syncpb.StationMessage_Error:
		return errors.New(body.Error.Message)
	default:
		return unexpectedStationFrameError("handling command stream")
	}
}

func (c *Client) sendReplayData(ctx context.Context, stream internalsyncv2.StationMessageStream, info common.StationInfoStruct, itemsLatest map[string]int64) error {
	var points []*syncpb.DataPoint
	for _, dv := range info.Devices {
		for _, itemName := range dv {
			ds, err := c.deps.GetDataHistory(itemName, itemsLatest[itemName], 0)
			if err != nil {
				return err
			}
			for _, d := range ds {
				points = append(points, &syncpb.DataPoint{
					ItemName: itemName,
					Value:    d.Value,
					UnixMs:   d.Millisecond.ToInt64(),
					Kind:     syncpb.DataKind_DATA_KIND_NORMAL,
				})
				if len(points) >= replayDataBatchSize {
					if err = c.sendMainFrame(ctx, stream, &syncpb.StationMessage{Body: &syncpb.StationMessage_DataBatch{
						DataBatch: &syncpb.DataBatch{
							Replay: true,
							Points: points,
						},
					}}); err != nil {
						return err
					}
					points = nil
				}
			}
		}
	}
	if len(points) > 0 {
		if err := c.sendMainFrame(ctx, stream, &syncpb.StationMessage{Body: &syncpb.StationMessage_DataBatch{
			DataBatch: &syncpb.DataBatch{
				Replay: true,
				Points: points,
			},
		}}); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) sendReplayStatus(ctx context.Context, stream internalsyncv2.StationMessageStream, latestStatusLogRowID int64) error {
	logs, err := c.deps.GetItemStatusLogAfter(latestStatusLogRowID)
	if err != nil {
		return err
	}
	if len(logs) == 0 {
		return nil
	}
	pbLogs := make([]*syncpb.ItemStatusLog, 0, len(logs))
	for _, l := range logs {
		pbLogs = append(pbLogs, &syncpb.ItemStatusLog{
			RowId:           l.RowId,
			ItemName:        l.ItemName,
			Status:          l.Status,
			ChangedAtUnixMs: l.ChangedAt.ToInt64(),
		})
	}
	return c.sendMainFrame(ctx, stream, &syncpb.StationMessage{Body: &syncpb.StationMessage_ItemStatusBatch{
		ItemStatusBatch: &syncpb.ItemStatusBatch{
			Replay: true,
			Logs:   pbLogs,
		},
	}})
}

func (c *Client) forwardRealtimeData(ctx context.Context, stream internalsyncv2.StationMessageStream, val any) error {
	msg, ok := val.(common.SendMsgStruct)
	if !ok {
		return nil
	}

	frame, ok := buildRealtimeFrame(msg)
	if !ok {
		return nil
	}
	return c.sendMainFrame(ctx, stream, frame)
}
