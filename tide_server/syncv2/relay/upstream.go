package syncv2relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"tide/common"
	internalsyncv2 "tide/internal/syncv2"
	"tide/pkg/custype"
	syncpb "tide/pkg/pb/syncproto"
	"tide/pkg/pubsub"
	"tide/tide_server/auth"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

func (s *UpstreamServer) StreamRelay(ctx context.Context, stream internalsyncv2.RelayMessageStream, authenticatedUsername string) error {
	if s == nil {
		return errors.New("nil upstream server")
	}
	if s.Store == nil || s.Auth.UserManager == nil || s.Auth.Permission == nil || s.Frames == nil {
		return errors.New("missing deps")
	}

	log := s.Logger
	if log == nil {
		log = slog.Default()
	}

	hello, err := recvDownstreamHello(stream)
	if err != nil {
		return err
	}
	username := strings.TrimSpace(hello.Username)
	if username == "" {
		return errors.New("empty username in downstream_hello")
	}
	if username != authenticatedUsername {
		return errors.New("username mismatch")
	}

	log.Info("v2 upstream: downstream connected", "username", username, "protocol_version", hello.ProtocolVersion)

	var permissions common.UUIDStringsMap
	user, err := s.Auth.UserManager.GetUser(username)
	if err != nil {
		log.Error("v2 upstream: failed to get user info", "username", username, "error", err)
		return fmt.Errorf("failed to get user info: %w", err)
	}
	if user.Role == auth.NormalUser {
		permissions, err = s.Auth.Permission.GetPermissions(username)
		if err != nil {
			log.Error("v2 upstream: failed to get permissions", "username", username, "error", err)
			return fmt.Errorf("failed to get user permissions: %w", err)
		}
	}

	var permissionTopics pubsub.TopicSet
	if permissions != nil {
		permissionTopics = uuidStringsMapToTopics(permissions)
	}

	configCh, dataCh, stop := s.Frames.Subscribe(ctx, username, permissionTopics)
	defer stop()

	sendFrame := func(frame *syncpb.RelayMessage) error { return stream.Send(frame) }

	if err = s.runBootstrapSync(stream, sendFrame, permissionTopics, permissions); err != nil {
		return err
	}

	log.Info("v2 upstream: handshake complete, entering incremental sync", "username", username)
	return s.forwardIncremental(ctx, stream, username, configCh, dataCh, sendFrame, log)
}

func (s *UpstreamServer) forwardIncremental(
	ctx context.Context,
	stream internalsyncv2.RelayMessageStream,
	username string,
	configCh <-chan *syncpb.RelayMessage,
	dataCh <-chan *syncpb.RelayMessage,
	sendFrame func(*syncpb.RelayMessage) error,
	log *slog.Logger,
) error {
	recvErrCh := make(chan error, 1)
	go func() {
		for {
			frame, recvErr := stream.Recv()
			if recvErr != nil {
				recvErrCh <- recvErr
				return
			}
			switch body := frame.Body.(type) {
			case *syncpb.RelayMessage_Error:
				log.Warn("v2 upstream: downstream reported error", "username", username, "code", body.Error.Code, "message", body.Error.Message)
			default:
				log.Debug("v2 upstream: unknown frame from downstream", "username", username)
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			log.Info("v2 upstream: connection closed", "username", username)
			return ctx.Err()

		case recvErr := <-recvErrCh:
			log.Info("v2 upstream: downstream disconnected", "username", username, "error", recvErr)
			return recvErr

		case frame, ok := <-configCh:
			if !ok || frame == nil {
				return errors.New("config subscriber closed")
			}
			if err := sendFrame(frame); err != nil {
				return err
			}

		case frame, ok := <-dataCh:
			if !ok || frame == nil {
				return errors.New("data subscriber closed")
			}
			if err := sendFrame(frame); err != nil {
				return err
			}
		}
	}
}

func (s *UpstreamServer) runBootstrapSync(
	stream internalsyncv2.RelayMessageStream,
	sendFrame func(*syncpb.RelayMessage) error,
	permissionTopics pubsub.TopicSet,
	permissions common.UUIDStringsMap,
) error {
	if err := sendFrame(&syncpb.RelayMessage{Body: &syncpb.RelayMessage_UpstreamHello{
		UpstreamHello: &syncpb.RelayUpstreamHello{
			ServerVersion: "sync-v2-upstream",
		},
	}}); err != nil {
		return err
	}
	if err := s.sendAvailableItems(sendFrame, permissionTopics); err != nil {
		return err
	}
	if err := s.sendFullConfig(sendFrame); err != nil {
		return err
	}
	if err := s.handleMissStatusSync(stream, sendFrame); err != nil {
		return err
	}
	return s.handleMissDataSync(stream, sendFrame, permissions)
}

func recvDownstreamHello(stream internalsyncv2.RelayMessageStream) (*syncpb.RelayDownstreamHello, error) {
	first, err := stream.Recv()
	if err != nil {
		return nil, err
	}
	hello, ok := first.Body.(*syncpb.RelayMessage_DownstreamHello)
	if !ok || hello.DownstreamHello == nil {
		return nil, errors.New("first frame must be downstream_hello")
	}
	if strings.TrimSpace(hello.DownstreamHello.Username) == "" {
		return nil, errors.New("username is required")
	}
	return hello.DownstreamHello, nil
}

func (s *UpstreamServer) sendAvailableItems(sendFrame func(*syncpb.RelayMessage) error, permissionTopics pubsub.TopicSet) error {
	localAvail, err := s.Store.GetAvailableItems()
	if err != nil {
		return fmt.Errorf("failed to get available items: %w", err)
	}

	stationsMap := make(map[string]*syncpb.RelayAvailableItemList)
	for _, si := range localAvail {
		if permissionTopics != nil {
			if _, ok := permissionTopics[si]; !ok {
				continue
			}
		}
		stationKey := si.StationId.String()
		if stationsMap[stationKey] == nil {
			stationsMap[stationKey] = &syncpb.RelayAvailableItemList{}
		}
		stationsMap[stationKey].ItemNames = append(stationsMap[stationKey].ItemNames, si.ItemName)
	}

	return sendFrame(&syncpb.RelayMessage{Body: &syncpb.RelayMessage_AvailableItems{
		AvailableItems: &syncpb.RelayAvailableItems{Stations: stationsMap},
	}})
}

func (s *UpstreamServer) sendFullConfig(sendFrame func(*syncpb.RelayMessage) error) error {
	stationsFull, err := s.Store.GetStationsFullInfo()
	if err != nil {
		return fmt.Errorf("failed to get stations full info: %w", err)
	}
	deviceRecords, err := s.Store.GetDeviceRecords()
	if err != nil {
		return fmt.Errorf("failed to get device records: %w", err)
	}

	var pbStations []*syncpb.RelayStationFull
	for _, sf := range stationsFull {
		pbStations = append(pbStations, dbStationFullToProto(sf))
	}
	var pbRecords []*syncpb.RelayDeviceRecord
	for _, dr := range deviceRecords {
		pbRecords = append(pbRecords, dbDeviceRecordToProto(dr))
	}

	return sendFrame(&syncpb.RelayMessage{Body: &syncpb.RelayMessage_ConfigBatch{
		ConfigBatch: &syncpb.RelayConfigBatch{
			FullSync:      true,
			Stations:      pbStations,
			DeviceRecords: pbRecords,
		},
	}})
}

func (s *UpstreamServer) handleMissStatusSync(stream internalsyncv2.RelayMessageStream, sendFrame func(*syncpb.RelayMessage) error) error {
	frame, err := stream.Recv()
	if err != nil {
		return err
	}
	statusLatest, ok := frame.Body.(*syncpb.RelayMessage_StationsStatusLatest)
	if !ok || statusLatest.StationsStatusLatest == nil {
		return errors.New("expected stations_status_latest frame")
	}

	var events []*syncpb.RelayConfigEvent
	for stationIDStr, latestRowID := range statusLatest.StationsStatusLatest.Stations {
		stationID, parseErr := uuid.Parse(stationIDStr)
		if parseErr != nil {
			continue
		}

		logs, queryErr := s.Store.GetItemStatusLogs(stationID, latestRowID)
		if queryErr != nil {
			return fmt.Errorf("failed to get item status logs: %w", queryErr)
		}
		for _, log := range logs {
			payload, marshalErr := json.Marshal(common.FullItemStatusStruct{
				StationId:             stationID,
				Identifier:            "",
				RowIdItemStatusStruct: log,
			})
			if marshalErr != nil {
				continue
			}
			events = append(events, &syncpb.RelayConfigEvent{
				Type:    MsgMissItemStatus,
				Payload: payload,
			})
		}
	}

	return sendFrame(&syncpb.RelayMessage{Body: &syncpb.RelayMessage_ConfigBatch{
		ConfigBatch: &syncpb.RelayConfigBatch{
			FullSync: false,
			Events:   events,
		},
	}})
}

func (s *UpstreamServer) handleMissDataSync(stream internalsyncv2.RelayMessageStream, sendFrame func(*syncpb.RelayMessage) error, permissions common.UUIDStringsMap) error {
	frame, err := stream.Recv()
	if err != nil {
		return err
	}
	itemsLatest, ok := frame.Body.(*syncpb.RelayMessage_StationsItemsLatest)
	if !ok || itemsLatest.StationsItemsLatest == nil {
		return errors.New("expected stations_items_latest frame")
	}

	effectivePerms := permissions
	if effectivePerms == nil {
		items, queryErr := s.Store.GetItemsAllStations()
		if queryErr != nil {
			return fmt.Errorf("failed to get all items: %w", queryErr)
		}
		effectivePerms = make(common.UUIDStringsMap)
		for _, item := range items {
			effectivePerms[item.StationId] = append(effectivePerms[item.StationId], item.Name)
		}
	}

	for stationID, itemNames := range effectivePerms {
		downstreamLatest := itemsLatest.StationsItemsLatest.Stations[stationID.String()]
		for _, itemName := range itemNames {
			var afterMs int64
			if downstreamLatest != nil {
				afterMs = downstreamLatest.LatestUnixMs[itemName]
			}
			if afterMs <= 0 {
				continue
			}

			ds, queryErr := s.Store.GetDataHistory(stationID, itemName, custype.UnixMs(afterMs))
			if queryErr != nil {
				var pgErr *pgconn.PgError
				if errors.As(queryErr, &pgErr) && pgErr.Code == "42P01" {
					continue
				}
				return fmt.Errorf("failed to get data history: %w", queryErr)
			}
			if len(ds) == 0 {
				continue
			}

			var points []*syncpb.DataPoint
			for _, d := range ds {
				points = append(points, &syncpb.DataPoint{
					ItemName: itemName,
					Value:    d.Value,
					UnixMs:   d.Millisecond.ToInt64(),
					Kind:     syncpb.DataKind_DATA_KIND_NORMAL,
				})
			}
			if err := sendFrame(&syncpb.RelayMessage{Body: &syncpb.RelayMessage_DataBatch{
				DataBatch: &syncpb.RelayDataBatch{
					StationId: stationID.String(),
					DataType:  MsgMissData,
					Points:    points,
				},
			}}); err != nil {
				return err
			}
		}
	}

	return sendFrame(&syncpb.RelayMessage{Body: &syncpb.RelayMessage_DataBatch{
		DataBatch: &syncpb.RelayDataBatch{},
	}})
}
