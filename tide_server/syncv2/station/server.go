package syncv2station

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"tide/common"
	internalsyncv2 "tide/internal/syncv2"
	"tide/pkg/custype"

	syncpb "tide/pkg/pb/syncproto"

	"github.com/google/uuid"
)

type Server struct {
	Store      Store
	InfoSyncer InfoSyncer
	Notifier   Notifier
	Logger     *slog.Logger
	// HandshakeTimeout 覆盖默认的握手时限（defaultHandshakeTimeout）；零值表示用默认值。
	HandshakeTimeout time.Duration

	reg registry
}

var ErrStationNotConnected = errors.New("station not connected")

// defaultHandshakeTimeout 是从连接建立到握手完成（服务端发出 StatusLatest）的时限。
// 握手阶段的每一步都在等对端的下一帧；没有时限的话，一个只发 ClientHello 就不再
// 说话的对端会一直占着该站点的登记槽位，真正的站点会被"station already connected"拒之门外。
const defaultHandshakeTimeout = 60 * time.Second

type commandStreamOpener func() (internalsyncv2.StationMessageStream, error)

func (s *Server) StreamStation(ctx context.Context, stream internalsyncv2.StationMessageStream, openCommandStream commandStreamOpener, remoteAddr string) error {
	if s == nil {
		return errors.New("nil server")
	}
	if s.Store == nil || s.InfoSyncer == nil || s.Notifier == nil {
		return errors.New("missing deps")
	}
	if openCommandStream == nil {
		return errors.New("missing command stream opener")
	}

	log := s.Logger
	if log == nil {
		log = slog.Default()
	}

	// 握手阶段给读操作设截止时间：超时后阻塞中的 Recv 返回错误，本函数随之退出并释放
	// 登记槽位。不能靠关闭流来打断——yamux 流的 Close 只是半关闭，不会唤醒本端的读。
	handshakeTimeout := s.HandshakeTimeout
	if handshakeTimeout <= 0 {
		handshakeTimeout = defaultHandshakeTimeout
	}
	_ = stream.SetReadDeadline(time.Now().Add(handshakeTimeout))

	hello, err := recvHello(stream)
	if err != nil {
		return err
	}

	stationID, err := s.Store.StationIDByIdentifier(hello.StationIdentifier)
	if err != nil {
		return fmt.Errorf("station not found: %w", err)
	}

	conn := newStationConn(openCommandStream, ctx.Done(), stream)
	if _, loaded := s.reg.LoadOrStore(stationID, conn); loaded {
		return errors.New("station already connected")
	}
	defer s.reg.Delete(stationID)

	if err = stream.Send(&syncpb.StationMessage{Body: &syncpb.StationMessage_ServerHello{
		ServerHello: &syncpb.ServerHello{
			ServerVersion: "sync-v2",
		},
	}}); err != nil {
		return err
	}

	stationInfo, err := recvStationInfo(stream)
	if err != nil {
		return err
	}
	if stationInfo.Identifier != hello.StationIdentifier {
		return errors.New("station identifier mismatch")
	}

	defer func() {
		_ = s.updateStationStatusAndNotify(stationID, stationInfo.Identifier, common.Disconnected)
		log.Info("v2 station disconnected", "identifier", stationInfo.Identifier, "remote", remoteAddr)
	}()

	if err := s.updateStationStatusAndNotify(stationID, stationInfo.Identifier, common.Normal); err != nil {
		return err
	}

	if err = s.Store.SetStationIP(stationID, remoteAddr); err != nil {
		return fmt.Errorf("failed to update station ip: %w", err)
	}
	if err = s.InfoSyncer.SyncStationInfo(stationID, stationInfo); err != nil {
		return err
	}

	log.Info("v2 station connected", "identifier", stationInfo.Identifier, "remote", remoteAddr)

	if err = s.applyClientStatusLatest(stream, stationID); err != nil {
		return err
	}

	itemsLatest, err := s.Store.ItemsLatest(stationID, stationInfo.Devices)
	if err != nil {
		return fmt.Errorf("failed to get item latest: %w", err)
	}
	if err = stream.Send(&syncpb.StationMessage{Body: &syncpb.StationMessage_ItemsLatest{
		ItemsLatest: &syncpb.ItemsLatest{LatestUnixMs: itemsLatest},
	}}); err != nil {
		return err
	}

	latestStatusRowID, err := s.Store.LatestStatusLogRowID(stationID)
	if err != nil {
		return fmt.Errorf("failed to get latest status row id: %w", err)
	}
	if err = stream.Send(&syncpb.StationMessage{Body: &syncpb.StationMessage_StatusLatest{
		StatusLatest: &syncpb.StatusLatest{LatestRowId: latestStatusRowID},
	}}); err != nil {
		return err
	}
	// 握手完成，取消读截止时间；之后的空闲由 yamux keepalive 负责探测。
	// 取消之后再检查一次断开标记：Disconnect 可能刚好在握手期间设置过"立即超时"，
	// 不检查的话会被这里的取消覆盖掉。
	_ = stream.SetReadDeadline(time.Time{})
	if conn.disconnected.Load() {
		return errStationDisconnected
	}

	for {
		frame, recvErr := stream.Recv()
		if recvErr != nil {
			if conn.disconnected.Load() {
				return errStationDisconnected
			}
			return recvErr
		}

		switch body := frame.Body.(type) {
		case *syncpb.StationMessage_DataBatch:
			if err = s.handleDataBatch(stationID, body.DataBatch); err != nil {
				return err
			}
		case *syncpb.StationMessage_ItemStatusBatch:
			if err = s.handleItemStatusBatch(stationID, stationInfo.Identifier, body.ItemStatusBatch); err != nil {
				return err
			}
		case *syncpb.StationMessage_RpiStatus:
			tm := custype.UnixMs(body.RpiStatus.UnixMs)
			if err = s.Store.SaveRpiStatus(stationID, body.RpiStatus.CpuTemp, tm.ToTime()); err != nil {
				return err
			}
		case *syncpb.StationMessage_StationInfo:
			if err := s.InfoSyncer.SyncStationInfo(stationID, internalsyncv2.PBToStationInfo(body.StationInfo)); err != nil {
				return err
			}
		default:
			log.Warn("unknown v2 station frame", "station", hello.StationIdentifier)
		}
	}
}

func (s *Server) RequestSnapshot(stationID uuid.UUID, cameraName string, timeout time.Duration) ([]byte, error) {
	if strings.TrimSpace(cameraName) == "" {
		return nil, errors.New("empty camera name")
	}
	conn, ok := s.reg.Load(stationID)
	if !ok {
		return nil, ErrStationNotConnected
	}
	return conn.requestSnapshot(cameraName, timeout)
}

// Disconnect 断开指定站点当前的 v2 连接（站点被删除时调用）。站点未连接时是空操作。
// 返回是否确实有连接被断开。
func (s *Server) Disconnect(stationID uuid.UUID) bool {
	if s == nil {
		return false
	}
	conn, ok := s.reg.Load(stationID)
	if !ok {
		return false
	}
	conn.close()
	return true
}

func (s *Server) updateStationStatusAndNotify(stationID uuid.UUID, identifier string, status common.Status) error {
	now := custype.ToUnixMs(time.Now())
	changed, err := s.Store.UpdateStationStatus(stationID, status, now.ToTime())
	if err != nil {
		return err
	}
	if changed {
		s.Notifier.PublishStationStatus(common.StationStatusStruct{
			StationId:          stationID,
			Identifier:         identifier,
			StatusChangeStruct: common.StatusChangeStruct{Status: status, ChangedAt: now},
		})
	}
	return nil
}

func recvHello(stream internalsyncv2.StationMessageStream) (*syncpb.ClientHello, error) {
	first, err := stream.Recv()
	if err != nil {
		return nil, err
	}
	hello, ok := first.Body.(*syncpb.StationMessage_ClientHello)
	if !ok || hello.ClientHello == nil {
		return nil, errors.New("first frame must be client_hello")
	}
	if strings.TrimSpace(hello.ClientHello.StationIdentifier) == "" {
		return nil, errors.New("station_identifier is required")
	}
	return hello.ClientHello, nil
}

func recvStationInfo(stream internalsyncv2.StationMessageStream) (common.StationInfoStruct, error) {
	first, err := stream.Recv()
	if err != nil {
		return common.StationInfoStruct{}, err
	}
	info, ok := first.Body.(*syncpb.StationMessage_StationInfo)
	if !ok || info.StationInfo == nil {
		return common.StationInfoStruct{}, errors.New("second frame must be station_info")
	}
	ret := internalsyncv2.PBToStationInfo(info.StationInfo)
	if ret.Identifier == "" || len(ret.Devices) == 0 {
		return common.StationInfoStruct{}, errors.New("invalid station_info content")
	}
	return ret, nil
}

func recvClientStatusLatest(stream internalsyncv2.StationMessageStream) (*syncpb.ClientStatusLatest, error) {
	frame, err := stream.Recv()
	if err != nil {
		return nil, err
	}
	body, ok := frame.Body.(*syncpb.StationMessage_ClientStatusLatest)
	if !ok || body.ClientStatusLatest == nil {
		return nil, errors.New("expected client_status_latest frame")
	}
	return body.ClientStatusLatest, nil
}

// applyClientStatusLatest reconciles the server's item_status_log with the client's local watermark.
// If the client reports an empty log while the server has rows, or if the row identified by the
// client's latest_row_id has a different changed_at on the server, the server treats the local SQLite
// as recreated and prunes all rows for the station. When the watermarks match content-wise, any
// trailing rows beyond the client's max are pruned defensively.
func (s *Server) applyClientStatusLatest(stream internalsyncv2.StationMessageStream, stationID uuid.UUID) error {
	clientLatest, err := recvClientStatusLatest(stream)
	if err != nil {
		return err
	}
	serverLatest, err := s.Store.LatestStatusLogRowID(stationID)
	if err != nil {
		return fmt.Errorf("failed to get latest status row id: %w", err)
	}

	if clientLatest.LatestRowId == 0 {
		if serverLatest > 0 {
			if err := s.Store.PruneItemStatusLog(stationID, 0, true); err != nil {
				return fmt.Errorf("failed to prune item status log: %w", err)
			}
		}
		return nil
	}

	changedAt, found, err := s.Store.ItemStatusLogChangedAt(stationID, clientLatest.LatestRowId)
	if err != nil {
		return fmt.Errorf("failed to inspect item status log: %w", err)
	}
	if !found {
		return nil
	}
	if custype.ToUnixMs(changedAt).ToInt64() != clientLatest.LatestChangedAtUnixMs {
		if err := s.Store.PruneItemStatusLog(stationID, 0, true); err != nil {
			return fmt.Errorf("failed to prune item status log: %w", err)
		}
		return nil
	}
	if serverLatest > clientLatest.LatestRowId {
		if err := s.Store.PruneItemStatusLog(stationID, clientLatest.LatestRowId, false); err != nil {
			return fmt.Errorf("failed to prune item status log tail: %w", err)
		}
	}
	return nil
}

func (s *Server) handleDataBatch(stationID uuid.UUID, batch *syncpb.DataBatch) error {
	for _, point := range batch.Points {
		tm := custype.UnixMs(point.UnixMs)
		stationItem := common.StationItemStruct{StationId: stationID, ItemName: point.ItemName}

		if point.Kind == syncpb.DataKind_DATA_KIND_GPIO {
			_, _ = s.Store.UpdateItemStatus(stationID, point.ItemName, common.NoStatus, tm.ToTime())
		}

		inserted, err := s.Store.SaveDataHistory(stationID, point.ItemName, point.Value, tm.ToTime())
		if err != nil {
			return err
		}
		if !inserted {
			continue
		}

		data := common.DataTimeStruct{Value: point.Value, Millisecond: tm}
		if batch.Replay {
			s.Notifier.PublishMissData(stationItem, data)
			continue
		}
		s.Notifier.PublishRealtimeData(stationItem, data, point.Kind == syncpb.DataKind_DATA_KIND_GPIO)
	}
	return nil
}

func (s *Server) handleItemStatusBatch(stationID uuid.UUID, identifier string, batch *syncpb.ItemStatusBatch) error {
	if batch.Replay {
		latestByItem := make(map[string]common.RowIdItemStatusStruct)
		for _, msg := range batch.Logs {
			statusLog := pbToItemStatusLog(msg)
			latestByItem[msg.ItemName] = statusLog
		}
		for _, v := range latestByItem {
			_, err := s.Store.UpdateItemStatus(stationID, v.ItemName, v.Status, v.ChangedAt.ToTime())
			if err != nil {
				return err
			}
		}

		for _, msg := range batch.Logs {
			statusLog := pbToItemStatusLog(msg)
			inserted, err := s.Store.SaveItemStatusLog(stationID, statusLog.RowId, statusLog.ItemName, statusLog.Status, statusLog.ChangedAt.ToTime())
			if err != nil {
				return err
			}
			if inserted {
				s.Notifier.PublishMissItemStatus(common.FullItemStatusStruct{
					StationId:             stationID,
					Identifier:            identifier,
					RowIdItemStatusStruct: statusLog,
				})
			}
		}
		return nil
	}

	for _, msg := range batch.Logs {
		statusLog := pbToItemStatusLog(msg)
		inserted, err := s.Store.UpdateAndSaveStatusLog(stationID, statusLog.RowId, statusLog.ItemName, statusLog.Status, statusLog.ChangedAt.ToTime())
		if err != nil {
			return err
		}
		if inserted {
			s.Notifier.PublishUpdateItemStatus(common.FullItemStatusStruct{
				StationId:             stationID,
				Identifier:            identifier,
				RowIdItemStatusStruct: statusLog,
			})
		}
	}
	return nil
}

func pbToItemStatusLog(msg *syncpb.ItemStatusLog) common.RowIdItemStatusStruct {
	return common.RowIdItemStatusStruct{
		RowId: msg.RowId,
		ItemStatusStruct: common.ItemStatusStruct{
			ItemName: msg.ItemName,
			StatusChangeStruct: common.StatusChangeStruct{
				Status:    msg.Status,
				ChangedAt: custype.UnixMs(msg.ChangedAtUnixMs),
			},
		},
	}
}

var errStationDisconnected = errors.New("station disconnected by server")

type stationConn struct {
	openCommandStream commandStreamOpener
	done              <-chan struct{}
	mainStream        internalsyncv2.StationMessageStream
	// disconnected 由 close 置位，StreamStation 据此区分"被服务端断开"和普通的读错误。
	disconnected atomic.Bool
}

func newStationConn(openCommandStream commandStreamOpener, done <-chan struct{}, mainStream internalsyncv2.StationMessageStream) *stationConn {
	return &stationConn{
		openCommandStream: openCommandStream,
		done:              done,
		mainStream:        mainStream,
	}
}

// close 让 StreamStation 的接收循环立即退出（调用方随后会关闭整个会话）。
// 用"已过期的读截止时间"而不是 Close：yamux 流的 Close 不会唤醒阻塞中的读。
func (c *stationConn) close() {
	c.disconnected.Store(true)
	_ = c.mainStream.SetReadDeadline(time.Now())
}

func (c *stationConn) requestSnapshot(cameraName string, timeout time.Duration) ([]byte, error) {
	cmdStream, err := c.openCommandStream()
	if err != nil {
		return nil, err
	}
	defer func() { _ = cmdStream.Close() }()

	if err := cmdStream.Send(&syncpb.StationMessage{Body: &syncpb.StationMessage_CameraSnapshotRequest{
		CameraSnapshotRequest: &syncpb.CameraSnapshotRequest{
			CameraName: cameraName,
		},
	}}); err != nil {
		return nil, err
	}

	type recvResult struct {
		frame *syncpb.StationMessage
		err   error
	}
	recvCh := make(chan recvResult, 1)
	go func() {
		frame, recvErr := cmdStream.Recv()
		recvCh <- recvResult{frame: frame, err: recvErr}
	}()

	var timer *time.Timer
	if timeout > 0 {
		timer = time.NewTimer(timeout)
		defer timer.Stop()
	}

	for {
		select {
		case <-c.done:
			return nil, errors.New("station connection closed")
		case <-timerChan(timer):
			return nil, context.DeadlineExceeded
		case r := <-recvCh:
			if r.err != nil {
				if errors.Is(r.err, io.EOF) {
					return nil, errors.New("snapshot stream closed")
				}
				return nil, r.err
			}
			body, ok := r.frame.Body.(*syncpb.StationMessage_CameraSnapshotResponse)
			if !ok || body.CameraSnapshotResponse == nil {
				return nil, errors.New("expected camera_snapshot_response")
			}
			if body.CameraSnapshotResponse.Error != "" {
				return nil, errors.New(body.CameraSnapshotResponse.Error)
			}
			if len(body.CameraSnapshotResponse.Data) == 0 {
				return nil, errors.New("empty snapshot")
			}
			return body.CameraSnapshotResponse.Data, nil
		}
	}
}

func timerChan(timer *time.Timer) <-chan time.Time {
	if timer == nil {
		return nil
	}
	return timer.C
}
