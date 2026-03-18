package controller

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"tide/common"
	internalsyncv2 "tide/internal/syncv2"
	syncpb "tide/pkg/pb/syncproto"
	"tide/pkg/pubsub"
	"tide/tide_server/global"
	syncv2relay "tide/tide_server/syncv2/relay"
	syncv2station "tide/tide_server/syncv2/station"

	"github.com/google/uuid"
)

var (
	v2StationServer  *syncv2station.Server
	v2StationHandler *syncv2station.Handler

	v2RelayHandler *syncv2relay.UpstreamHandler
)

func initSyncV2() {
	v2StationServer = &syncv2station.Server{
		Store:      syncv2station.DBStore{},
		InfoSyncer: syncV2StationInfoSyncer{},
		Notifier:   syncV2StationNotifier{},
		Logger:     slog.Default(),
	}
	v2StationHandler = &syncv2station.Handler{
		Enabled:       func() bool { return global.Config.SyncV2.Enabled },
		Server:        v2StationServer,
		MaxFrameBytes: internalsyncv2.DefaultMaxFrameBytes,
		Logger:        slog.Default(),
	}

	upstreamServer := &syncv2relay.UpstreamServer{
		Store:  syncv2relay.DBUpstreamStore{},
		Auth:   syncv2relay.UpstreamAuthDeps{UserManager: userManager, Permission: authorization},
		Frames: syncV2RelayFrameSource{},
		Logger: slog.Default(),
	}
	v2RelayHandler = &syncv2relay.UpstreamHandler{
		Enabled:             func() bool { return global.Config.SyncV2.Enabled },
		Server:              upstreamServer,
		MaxFrameBytes:       internalsyncv2.DefaultMaxFrameBytes,
		UsernameFromRequest: requestUsername,
		Logger:              slog.Default(),
	}
}

type syncV2StationInfoSyncer struct{}

func (syncV2StationInfoSyncer) SyncStationInfo(stationID uuid.UUID, info common.StationInfoStruct) error {
	if ok := SyncStationInfo(stationID, info); !ok {
		return errors.New("failed to sync station info")
	}
	return nil
}

type syncV2StationNotifier struct{}

func (syncV2StationNotifier) PublishMissData(stationItem common.StationItemStruct, data common.DataTimeStruct) {
	hub.Publish(BrokerMissingData, forwardDataStruct{
		Type:              kMsgMissData,
		StationItemStruct: stationItem,
		DataTimeStruct:    data,
	}, stationItem)
}

func (syncV2StationNotifier) PublishRealtimeData(stationItem common.StationItemStruct, data common.DataTimeStruct, gpio bool) {
	msgType := kMsgData
	if gpio {
		msgType = kMsgDataGpio
	}
	hub.PublishDelayedData(forwardDataStruct{
		Type:              msgType,
		StationItemStruct: stationItem,
		DataTimeStruct:    data,
	}, stationItem)
}

func (syncV2StationNotifier) PublishMissItemStatus(status common.FullItemStatusStruct) {
	hub.Publish(BrokerConfig, SendMsgStruct{Type: kMsgMissItemStatus, Body: status}, nil)
}

func (syncV2StationNotifier) PublishUpdateItemStatus(status common.FullItemStatusStruct) {
	hub.Publish(BrokerStatus, SendMsgStruct{Type: kMsgUpdateItemStatus, Body: status}, nil)
}

func (syncV2StationNotifier) PublishStationStatus(status common.StationStatusStruct) {
	hub.Publish(BrokerStatus, SendMsgStruct{Type: kMsgUpdateStationStatus, Body: status}, nil)
}

type syncV2RelayDownstreamNotifier struct{}

func (syncV2RelayDownstreamNotifier) PublishConfig(typeStr string, body any) {
	hub.Publish(BrokerConfig, SendMsgStruct{Type: typeStr, Body: body}, nil)
}

func (syncV2RelayDownstreamNotifier) PublishStatus(typeStr string, body any) {
	hub.Publish(BrokerStatus, SendMsgStruct{Type: typeStr, Body: body}, nil)
}

func (syncV2RelayDownstreamNotifier) BroadcastAvailableChange(items common.UUIDStringsMap) {
	hub.BroadcastAvailableChange(items)
}

func (syncV2RelayDownstreamNotifier) PublishMissData(topic common.StationItemStruct, data common.DataTimeStruct) {
	hub.Publish(BrokerMissingData, forwardDataStruct{
		Type:              kMsgMissData,
		StationItemStruct: topic,
		DataTimeStruct:    data,
	}, topic)
}

func (syncV2RelayDownstreamNotifier) PublishData(topic common.StationItemStruct, data common.DataTimeStruct, gpio bool) {
	msgType := kMsgData
	if gpio {
		msgType = kMsgDataGpio
	}
	hub.Publish(BrokerData, forwardDataStruct{
		Type:              msgType,
		StationItemStruct: topic,
		DataTimeStruct:    data,
	}, topic)
}

type syncV2RelayFrameSource struct{}

func (syncV2RelayFrameSource) Subscribe(ctx context.Context, username string, permissionTopics pubsub.TopicSet) (config <-chan *syncpb.RelayMessage, data <-chan *syncpb.RelayMessage, stop func()) {
	configCtx, configCancel := context.WithCancel(ctx)
	dataCtx, dataCancel := context.WithCancel(ctx)

	configSubscriber := pubsub.NewSubscriber(10000, configCancel)
	dataSubscriber := pubsub.NewSubscriber(10000, dataCancel)
	hub.Subscribe(BrokerConfig, configSubscriber, nil)
	hub.Subscribe(BrokerStatus, configSubscriber, nil)
	hub.Subscribe(BrokerData, dataSubscriber, permissionTopics)
	hub.Subscribe(BrokerMissingData, dataSubscriber, permissionTopics)

	hub.TrackSubscriber(username, configSubscriber, connTypeSyncConfig)
	hub.TrackSubscriber(username, dataSubscriber, connTypeSyncData)

	configOut := make(chan *syncpb.RelayMessage, 10000)
	dataOut := make(chan *syncpb.RelayMessage, 10000)

	go func() {
		defer close(configOut)
		for {
			select {
			case <-configCtx.Done():
				return
			case raw, ok := <-configSubscriber.Ch:
				if !ok {
					return
				}
				frame := relayConfigMessageToFrame(raw)
				if frame == nil {
					continue
				}
				select {
				case configOut <- frame:
				case <-configCtx.Done():
					return
				}
			}
		}
	}()

	go func() {
		defer close(dataOut)
		for {
			select {
			case <-dataCtx.Done():
				return
			case raw, ok := <-dataSubscriber.Ch:
				if !ok {
					return
				}
				frame := relayDataMessageToFrame(raw)
				if frame == nil {
					continue
				}
				select {
				case dataOut <- frame:
				case <-dataCtx.Done():
					return
				}
			}
		}
	}()

	stop = func() {
		configCancel()
		dataCancel()

		hub.UntrackSubscriber(username, configSubscriber)
		hub.UntrackSubscriber(username, dataSubscriber)

		hub.Unsubscribe(BrokerConfig, configSubscriber)
		hub.Unsubscribe(BrokerStatus, configSubscriber)
		hub.Unsubscribe(BrokerData, dataSubscriber)
		hub.Unsubscribe(BrokerMissingData, dataSubscriber)
	}

	return configOut, dataOut, stop
}

func relayConfigMessageToFrame(val any) *syncpb.RelayMessage {
	msg, ok := val.(SendMsgStruct)
	if !ok {
		slog.Error("v2 upstream: config message type assertion failed")
		return nil
	}

	switch msg.Type {
	case kMsgUpdateItemStatus, kMsgMissItemStatus:
		if statusInfo, ok := msg.Body.(common.FullItemStatusStruct); ok {
			return &syncpb.RelayMessage{Body: &syncpb.RelayMessage_StatusEvent{
				StatusEvent: &syncpb.RelayStatusEvent{
					StationId:       statusInfo.StationId.String(),
					Identifier:      statusInfo.Identifier,
					RowId:           statusInfo.RowId,
					ItemName:        statusInfo.ItemName,
					Status:          statusInfo.Status,
					ChangedAtUnixMs: statusInfo.ChangedAt.ToInt64(),
				},
			}}
		}

	case kMsgUpdateStationStatus:
		if stationStatus, ok := msg.Body.(common.StationStatusStruct); ok {
			return &syncpb.RelayMessage{Body: &syncpb.RelayMessage_StatusEvent{
				StatusEvent: &syncpb.RelayStatusEvent{
					StationId:       stationStatus.StationId.String(),
					Identifier:      stationStatus.Identifier,
					Status:          stationStatus.Status,
					ChangedAtUnixMs: stationStatus.ChangedAt.ToInt64(),
				},
			}}
		}
	}

	payload, err := json.Marshal(msg.Body)
	if err != nil {
		slog.Error("v2 upstream: marshal config payload failed", "type", msg.Type, "error", err)
		return nil
	}
	return &syncpb.RelayMessage{Body: &syncpb.RelayMessage_ConfigBatch{
		ConfigBatch: &syncpb.RelayConfigBatch{
			FullSync: false,
			Events: []*syncpb.RelayConfigEvent{
				{
					Type:    msg.Type,
					Payload: payload,
				},
			},
		},
	}}
}

func relayDataMessageToFrame(val any) *syncpb.RelayMessage {
	msg, ok := val.(forwardDataStruct)
	if !ok {
		slog.Error("v2 upstream: data message type assertion failed")
		return nil
	}

	kind := syncpb.DataKind_DATA_KIND_NORMAL
	if msg.Type == kMsgDataGpio {
		kind = syncpb.DataKind_DATA_KIND_GPIO
	}

	return &syncpb.RelayMessage{Body: &syncpb.RelayMessage_DataBatch{
		DataBatch: &syncpb.RelayDataBatch{
			StationId: msg.StationId.String(),
			DataType:  msg.Type,
			Points: []*syncpb.DataPoint{
				{
					ItemName: msg.ItemName,
					Value:    msg.Value,
					UnixMs:   msg.Millisecond.ToInt64(),
					Kind:     kind,
				},
			},
		},
	}}
}

func relayDownstreamDeps(state *upstreamSyncState) syncv2relay.DownstreamDeps {
	return syncv2relay.DownstreamDeps{
		AuthClient: state.httpClient,
		Store:      syncv2relay.DBDownstreamStore{},
		Notifier:   syncV2RelayDownstreamNotifier{},
		EditLock:   &editMu,

		MaxFrameBytes: internalsyncv2.DefaultMaxFrameBytes,
		Logger:        slog.Default(),
	}
}

func relayDownstreamConfig(upstream *upstreamSyncState) syncv2relay.DownstreamConfig {
	return syncv2relay.DownstreamConfig{
		UpstreamID: upstream.config.Id,
		BaseURL:    upstream.config.Url,
		Username:   upstream.config.Username,
		Password:   upstream.config.Password,
	}
}

func v2SnapshotOrErr(stationID uuid.UUID, cameraName string, timeout time.Duration) ([]byte, error) {
	if v2StationServer == nil {
		return nil, syncv2station.ErrStationNotConnected
	}
	return v2StationServer.RequestSnapshot(stationID, cameraName, timeout)
}
