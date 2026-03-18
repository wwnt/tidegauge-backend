package syncv2

import (
	"fmt"

	"tide/common"
	syncpb "tide/pkg/pb/syncproto"
)

func unexpectedStationFrameError(where string) error {
	return fmt.Errorf("unexpected frame while %s", where)
}

func buildRealtimeFrame(msg common.SendMsgStruct) (*syncpb.StationMessage, bool) {
	switch msg.Type {
	case common.MsgData, common.MsgGpioData:
		body, ok := msg.Body.(common.ItemNameDataTimeStruct)
		if !ok {
			return nil, false
		}
		kind := syncpb.DataKind_DATA_KIND_NORMAL
		if msg.Type == common.MsgGpioData {
			kind = syncpb.DataKind_DATA_KIND_GPIO
		}
		return &syncpb.StationMessage{Body: &syncpb.StationMessage_DataBatch{
			DataBatch: &syncpb.DataBatch{
				Points: []*syncpb.DataPoint{
					{
						ItemName: body.ItemName,
						Value:    body.Value,
						UnixMs:   body.Millisecond.ToInt64(),
						Kind:     kind,
					},
				},
			},
		}}, true

	case common.MsgItemStatus:
		body, ok := msg.Body.(common.RowIdItemStatusStruct)
		if !ok {
			return nil, false
		}
		return &syncpb.StationMessage{Body: &syncpb.StationMessage_ItemStatusBatch{
			ItemStatusBatch: &syncpb.ItemStatusBatch{
				Logs: []*syncpb.ItemStatusLog{
					{
						RowId:           body.RowId,
						ItemName:        body.ItemName,
						Status:          body.Status,
						ChangedAtUnixMs: body.ChangedAt.ToInt64(),
					},
				},
			},
		}}, true

	case common.MsgRpiStatus:
		body, ok := msg.Body.(common.RpiStatusTimeStruct)
		if !ok {
			return nil, false
		}
		return &syncpb.StationMessage{Body: &syncpb.StationMessage_RpiStatus{
			RpiStatus: &syncpb.RpiStatus{
				CpuTemp: body.CpuTemp,
				UnixMs:  body.Millisecond.ToInt64(),
			},
		}}, true
	}
	return nil, false
}

func buildSnapshotResponseFrame(req *syncpb.CameraSnapshotRequest, getCamera GetCameraFn, snapshot SnapshotFn) *syncpb.StationMessage {
	if req == nil || req.CameraName == "" {
		return &syncpb.StationMessage{Body: &syncpb.StationMessage_CameraSnapshotResponse{
			CameraSnapshotResponse: &syncpb.CameraSnapshotResponse{
				Error: "camera name is required",
			},
		}}
	}
	if getCamera == nil || snapshot == nil {
		return &syncpb.StationMessage{Body: &syncpb.StationMessage_CameraSnapshotResponse{
			CameraSnapshotResponse: &syncpb.CameraSnapshotResponse{
				Error: "snapshot dependencies are missing",
			},
		}}
	}

	snapshotURL, username, password, ok := getCamera(req.CameraName)
	if !ok {
		return &syncpb.StationMessage{Body: &syncpb.StationMessage_CameraSnapshotResponse{
			CameraSnapshotResponse: &syncpb.CameraSnapshotResponse{
				Error: "camera not found",
			},
		}}
	}

	bs, err := snapshot(snapshotURL, username, password)
	if err != nil {
		return &syncpb.StationMessage{Body: &syncpb.StationMessage_CameraSnapshotResponse{
			CameraSnapshotResponse: &syncpb.CameraSnapshotResponse{
				Error: err.Error(),
			},
		}}
	}

	return &syncpb.StationMessage{Body: &syncpb.StationMessage_CameraSnapshotResponse{
		CameraSnapshotResponse: &syncpb.CameraSnapshotResponse{
			Data: bs,
		},
	}}
}
