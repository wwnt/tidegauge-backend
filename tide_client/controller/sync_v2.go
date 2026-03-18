package controller

import (
	"context"
	"log/slog"

	"tide/common"
	"tide/tide_client/syncv2"

	"tide/pkg/pubsub"
	"tide/tide_client/db"
	"tide/tide_client/device/camera"
	"tide/tide_client/global"
)

// runSyncV2ClientOnce attempts to run one v2 sync session.
// Returns true if v2 is enabled and attempted (success or failure), false if v2 is disabled (caller should fall back to v1).
func runSyncV2ClientOnce(dataBroker *pubsub.Broker) bool {
	if !global.Config.SyncV2.Enabled || global.Config.SyncV2.Addr == "" {
		return false
	}

	addr := global.Config.SyncV2.Addr
	ctx := context.Background()
	client, err := syncv2.NewClient(
		syncv2.Config{
			Addr:              addr,
			StationIdentifier: global.Config.Identifier,
		},
		syncv2.Deps{
			StationInfoFn: func() common.StationInfoStruct {
				return stationInfo
			},
			GetDataHistory:        db.GetDataHistory,
			GetItemStatusLogAfter: db.GetItemStatusLogAfter,
			Subscribe:             dataBroker.Subscribe,
			Unsubscribe:           dataBroker.Unsubscribe,
			IngestLock:            &ingestMu,
			GetCamera: func(name string) (snapshotURL, username, password string, ok bool) {
				cam, ok := global.Config.Cameras.List[name]
				if !ok {
					return "", "", "", false
				}
				return cam.Snapshot, cam.Username, cam.Password, true
			},
			Snapshot: camera.OnvifSnapshot,
		},
	)
	if err != nil {
		slog.Error("invalid v2 sync client config", "addr", addr, "error", err)
		return true
	}

	if err = client.Run(ctx); err != nil {
		slog.Error("v2 sync client exited", "addr", addr, "error", err)
	}
	return true
}
