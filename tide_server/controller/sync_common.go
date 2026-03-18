package controller

import (
	"encoding/json"
	"io"
	"log/slog"
	"time"

	"tide/common"
	"tide/pkg/custype"
	"tide/tide_server/db"

	"github.com/google/uuid"
)

const (
	kMsgSyncStation           = "SyncStation"
	kMsgSyncStationCannotEdit = "SyncStationCannotEdit"
	kMsgDelUpstreamStation    = "DelUpstreamStation"
	kMsgSyncDevice            = "SyncDevice"
	kMsgDelDevice             = "DelDevice"
	kMsgSyncItem              = "SyncItem"
	kMsgDelItem               = "DelItem"
	kMsgEditDeviceRecord      = "EditDeviceRecord"
	kMsgUpdateAvailable       = "update_available"
	kMsgUpdateStationStatus   = "UpdateStationStatus"
	kMsgMissItemStatus        = "MissItemStatus"
	kMsgUpdateItemStatus      = "UpdateItemStatus"
	kMsgMissData              = "MissData"
	kMsgData                  = "data"
	kMsgDataGpio              = "data_gpio"
)

type forwardDataStruct struct {
	Type string
	common.StationItemStruct
	common.DataTimeStruct
}

type SendMsgStruct struct {
	Type string `json:"type"`
	Body any    `json:"body"`
}
type RcvMsgStruct struct {
	Type string          `json:"type"`
	Body json.RawMessage `json:"body"`
}

func updateStationStatusAndBroadcast(stationId uuid.UUID, identifier string, status common.Status) (ok bool) {
	now := custype.ToUnixMs(time.Now())
	if n, err := db.UpdateStationStatus(stationId, status, now.ToTime()); err != nil {
		slog.Error("Failed to update station status", "station_id", stationId, "status", status, "error", err)
		return
	} else if n > 0 {
		hub.Publish(BrokerStatus, SendMsgStruct{
			Type: kMsgUpdateStationStatus,
			Body: common.StationStatusStruct{
				StationId:          stationId,
				Identifier:         identifier,
				StatusChangeStruct: common.StatusChangeStruct{Status: status, ChangedAt: now},
			},
		}, nil)
	}
	return true
}

const (
	connTypeWebBrowser = 1 << iota
	connTypeSyncData
	connTypeSyncConfig
	connTypeAny = -1
)

// jsonWriter returns a write function that JSON-encodes values to w.
func jsonWriter(w io.Writer) func(any) error {
	return func(val any) error {
		b, err := json.Marshal(val)
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	}
}
