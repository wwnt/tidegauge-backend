package syncv2relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"tide/common"
	internalsyncv2 "tide/internal/syncv2"
	"tide/pkg/custype"
	syncpb "tide/pkg/pb/syncproto"

	"github.com/google/uuid"
)

func RunDownstream(ctx context.Context, cfg DownstreamConfig, deps DownstreamDeps) error {
	if deps.Store == nil || deps.Notifier == nil {
		return errors.New("missing deps")
	}
	if deps.AuthClient == nil {
		return errors.New("missing auth client")
	}

	log := deps.Logger
	if log == nil {
		log = slog.Default()
	}

	stream, relayURL, closeConn, err := openDownstreamStream(ctx, cfg, deps)
	if err != nil {
		return err
	}
	defer closeConn()

	go func() {
		<-ctx.Done()
		_ = stream.Close()
	}()

	if err = runDownstreamHandshake(stream, cfg); err != nil {
		return err
	}
	log.Info("v2 downstream: connected", "url", relayURL, "username", cfg.Username)

	if err = runDownstreamBootstrap(stream, cfg, deps); err != nil {
		return err
	}
	log.Info("v2 downstream: handshake complete, entering incremental sync", "url", relayURL)

	return runDownstreamIncremental(stream, cfg, deps, log)
}

func RunDownstreamWithRetry(ctx context.Context, cfg DownstreamConfig, deps DownstreamDeps, retryDelay time.Duration, onDisconnect func(error)) {
	for {
		err := RunDownstream(ctx, cfg, deps)
		if ctx.Err() != nil {
			return
		}
		if onDisconnect != nil {
			onDisconnect(err)
		}
		select {
		case <-time.After(retryDelay):
		case <-ctx.Done():
			return
		}
	}
}

func openDownstreamStream(ctx context.Context, cfg DownstreamConfig, deps DownstreamDeps) (internalsyncv2.RelayMessageStream, string, func(), error) {
	relayURL, err := internalsyncv2.RelayURL(cfg.BaseURL)
	if err != nil {
		return nil, "", nil, err
	}

	resp, err := deps.AuthClient.DoWithAuth(ctx, func(token string) (*http.Request, error) {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, relayURL, nil)
		if reqErr != nil {
			return nil, reqErr
		}
		// net/http/response.go: func isProtocolSwitchResponse()
		req.Header.Set("Connection", "Upgrade")
		req.Header.Set("Upgrade", "websocket")
		req.Header.Set("Authorization", "Bearer "+token)
		return req, nil
	})
	if err != nil {
		return nil, "", nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		_ = resp.Body.Close()
		return nil, "", nil, errors.New("upstream authentication failed")
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		_ = resp.Body.Close()
		return nil, "", nil, fmt.Errorf("unexpected upgrade status: %d", resp.StatusCode)
	}

	conn, ok := resp.Body.(io.ReadWriteCloser)
	if !ok {
		_ = resp.Body.Close()
		return nil, "", nil, errors.New("response body does not implement io.ReadWriteCloser")
	}

	maxFrameBytes := deps.MaxFrameBytes
	if maxFrameBytes <= 0 {
		maxFrameBytes = internalsyncv2.DefaultMaxFrameBytes
	}

	stream := internalsyncv2.NewRelayMessageStream(conn, maxFrameBytes)
	closeConn := func() {
		_ = stream.Close()
	}
	return stream, relayURL, closeConn, nil
}

func runDownstreamHandshake(stream internalsyncv2.RelayMessageStream, cfg DownstreamConfig) error {
	if err := stream.Send(&syncpb.RelayMessage{Body: &syncpb.RelayMessage_DownstreamHello{
		DownstreamHello: &syncpb.RelayDownstreamHello{
			Username:        cfg.Username,
			ProtocolVersion: internalsyncv2.ProtocolVersion,
		},
	}}); err != nil {
		return err
	}

	frame, err := stream.Recv()
	if err != nil {
		return err
	}
	if _, ok := frame.Body.(*syncpb.RelayMessage_UpstreamHello); !ok {
		return errors.New("expected upstream_hello")
	}
	return nil
}

func runDownstreamBootstrap(stream internalsyncv2.RelayMessageStream, cfg DownstreamConfig, deps DownstreamDeps) error {
	frame, err := stream.Recv()
	if err != nil {
		return err
	}
	availBody, ok := frame.Body.(*syncpb.RelayMessage_AvailableItems)
	if !ok {
		return errors.New("expected available_items")
	}
	if err = applyAvailableItems(cfg, deps, availBody.AvailableItems); err != nil {
		return err
	}

	frame, err = stream.Recv()
	if err != nil {
		return err
	}
	cfgBody, ok := frame.Body.(*syncpb.RelayMessage_ConfigBatch)
	if !ok || !cfgBody.ConfigBatch.FullSync {
		return errors.New("expected full config batch")
	}
	if err = applyFullConfig(cfg, deps, cfgBody.ConfigBatch); err != nil {
		return err
	}

	if err = syncMissStatus(stream, deps, cfgBody.ConfigBatch.Stations); err != nil {
		return err
	}
	if err = syncMissData(stream, deps); err != nil {
		return err
	}
	return nil
}

func runDownstreamIncremental(stream internalsyncv2.RelayMessageStream, cfg DownstreamConfig, deps DownstreamDeps, log *slog.Logger) error {
	for {
		frame, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		switch body := frame.Body.(type) {
		case *syncpb.RelayMessage_ConfigBatch:
			for _, evt := range body.ConfigBatch.Events {
				if applyErr := applyConfigEvent(cfg, deps, evt); applyErr != nil {
					log.Error("v2 downstream: apply config event failed", "type", evt.Type, "error", applyErr)
				}
			}
		case *syncpb.RelayMessage_StatusEvent:
			applyStatusEvent(deps, body.StatusEvent)
		case *syncpb.RelayMessage_DataBatch:
			applyDataBatch(deps, body.DataBatch)
		case *syncpb.RelayMessage_AvailableItems:
			if err := applyAvailableItems(cfg, deps, body.AvailableItems); err != nil {
				log.Error("v2 downstream: handle available items failed", "error", err)
			}
		case *syncpb.RelayMessage_Error:
			log.Error("v2 downstream: upstream reported error", "code", body.Error.Code, "message", body.Error.Message)
			if !body.Error.Retryable {
				return errors.New(body.Error.Message)
			}
		default:
			// ignore
		}
	}
}

func syncMissStatus(stream internalsyncv2.RelayMessageStream, deps DownstreamDeps, stations []*syncpb.RelayStationFull) error {
	stationsLatest := make(map[string]int64)
	for _, s := range stations {
		stationID, err := uuid.Parse(s.Id)
		if err != nil {
			continue
		}
		rowID, err := deps.Store.LatestStatusLogRowID(stationID)
		if err != nil {
			return err
		}
		stationsLatest[s.Id] = rowID
	}

	if err := stream.Send(&syncpb.RelayMessage{Body: &syncpb.RelayMessage_StationsStatusLatest{
		StationsStatusLatest: &syncpb.RelayStatusLatest{Stations: stationsLatest},
	}}); err != nil {
		return err
	}

	frame, err := stream.Recv()
	if err != nil {
		return err
	}
	cfgBatch, ok := frame.Body.(*syncpb.RelayMessage_ConfigBatch)
	if !ok {
		return errors.New("expected config batch with miss status logs")
	}

	for _, evt := range cfgBatch.ConfigBatch.Events {
		if evt.Type != MsgMissItemStatus {
			continue
		}
		var statusInfo common.FullItemStatusStruct
		if err := json.Unmarshal(evt.Payload, &statusInfo); err != nil {
			continue
		}
		if _, err := deps.Store.UpdateItemStatus(statusInfo.StationId, statusInfo.ItemName, statusInfo.Status, statusInfo.ChangedAt.ToTime()); err != nil {
			continue
		}
		inserted, err := deps.Store.SaveItemStatusLog(statusInfo.StationId, statusInfo.RowId, statusInfo.ItemName, statusInfo.Status, statusInfo.ChangedAt.ToTime())
		if err != nil {
			continue
		}
		if inserted {
			deps.Notifier.PublishConfig(MsgMissItemStatus, statusInfo)
		}
	}
	return nil
}

func syncMissData(stream internalsyncv2.RelayMessageStream, deps DownstreamDeps) error {
	items, err := deps.Store.GetAllItems()
	if err != nil {
		return err
	}

	stationsLatest := make(map[string]*syncpb.ItemsLatest)
	byStation := make(map[uuid.UUID][]string)
	for _, item := range items {
		byStation[item.StationId] = append(byStation[item.StationId], item.Name)
	}
	for stationID, itemNames := range byStation {
		latest, err := deps.Store.ItemsLatest(stationID, itemNames)
		if err != nil {
			return err
		}
		stationsLatest[stationID.String()] = &syncpb.ItemsLatest{LatestUnixMs: latest}
	}

	if err = stream.Send(&syncpb.RelayMessage{Body: &syncpb.RelayMessage_StationsItemsLatest{
		StationsItemsLatest: &syncpb.RelayItemsLatest{Stations: stationsLatest},
	}}); err != nil {
		return err
	}

	for {
		frame, err := stream.Recv()
		if err != nil {
			return err
		}
		dataBatch, ok := frame.Body.(*syncpb.RelayMessage_DataBatch)
		if !ok {
			return errors.New("unexpected frame during miss data sync")
		}
		batch := dataBatch.DataBatch
		if batch.StationId == "" && len(batch.Points) == 0 {
			return nil
		}
		stationID, parseErr := uuid.Parse(batch.StationId)
		if parseErr != nil {
			continue
		}
		for _, point := range batch.Points {
			if err = deps.Store.EnsureDataTable(point.ItemName); err != nil {
				return err
			}
			tm := custype.UnixMs(point.UnixMs)
			inserted, saveErr := deps.Store.SaveDataHistory(stationID, point.ItemName, point.Value, tm.ToTime())
			if saveErr != nil {
				return saveErr
			}
			if inserted {
				stationItem := common.StationItemStruct{StationId: stationID, ItemName: point.ItemName}
				deps.Notifier.PublishMissData(stationItem, common.DataTimeStruct{Value: point.Value, Millisecond: tm})
			}
		}
	}
}
