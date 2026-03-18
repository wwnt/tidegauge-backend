package syncv2

import (
	"errors"
	"testing"

	"tide/common"
	"tide/pkg/custype"
	syncpb "tide/pkg/pb/syncproto"

	"github.com/stretchr/testify/require"
)

func TestBuildRealtimeFrame_Data(t *testing.T) {
	frame, ok := buildRealtimeFrame(common.SendMsgStruct{
		Type: common.MsgData,
		Body: common.ItemNameDataTimeStruct{
			ItemName: "item1",
			DataTimeStruct: common.DataTimeStruct{
				Value:       1.23,
				Millisecond: custype.UnixMs(1000),
			},
		},
	})
	require.True(t, ok)
	body, ok := frame.Body.(*syncpb.StationMessage_DataBatch)
	require.True(t, ok)
	require.Len(t, body.DataBatch.Points, 1)
	require.Equal(t, syncpb.DataKind_DATA_KIND_NORMAL, body.DataBatch.Points[0].Kind)
}

func TestBuildRealtimeFrame_Gpio(t *testing.T) {
	frame, ok := buildRealtimeFrame(common.SendMsgStruct{
		Type: common.MsgGpioData,
		Body: common.ItemNameDataTimeStruct{
			ItemName: "item1",
			DataTimeStruct: common.DataTimeStruct{
				Value:       9.9,
				Millisecond: custype.UnixMs(2000),
			},
		},
	})
	require.True(t, ok)
	body, ok := frame.Body.(*syncpb.StationMessage_DataBatch)
	require.True(t, ok)
	require.Equal(t, syncpb.DataKind_DATA_KIND_GPIO, body.DataBatch.Points[0].Kind)
}

func TestBuildRealtimeFrame_ItemStatus(t *testing.T) {
	frame, ok := buildRealtimeFrame(common.SendMsgStruct{
		Type: common.MsgItemStatus,
		Body: common.RowIdItemStatusStruct{
			RowId: 7,
			ItemStatusStruct: common.ItemStatusStruct{
				ItemName: "item7",
				StatusChangeStruct: common.StatusChangeStruct{
					Status:    "Normal",
					ChangedAt: custype.UnixMs(3000),
				},
			},
		},
	})
	require.True(t, ok)
	body, ok := frame.Body.(*syncpb.StationMessage_ItemStatusBatch)
	require.True(t, ok)
	require.Len(t, body.ItemStatusBatch.Logs, 1)
	require.Equal(t, int64(7), body.ItemStatusBatch.Logs[0].RowId)
}

func TestBuildRealtimeFrame_RpiStatus(t *testing.T) {
	frame, ok := buildRealtimeFrame(common.SendMsgStruct{
		Type: common.MsgRpiStatus,
		Body: common.RpiStatusTimeStruct{
			RpiStatusStruct: common.RpiStatusStruct{
				CpuTemp: 45.6,
			},
			Millisecond: custype.UnixMs(4000),
		},
	})
	require.True(t, ok)
	body, ok := frame.Body.(*syncpb.StationMessage_RpiStatus)
	require.True(t, ok)
	require.Equal(t, float64(45.6), body.RpiStatus.CpuTemp)
	require.Equal(t, int64(4000), body.RpiStatus.UnixMs)
}

func TestBuildSnapshotResponseFrame_NilRequest(t *testing.T) {
	cameraLookup := fakeCameraLookup{ok: true}
	snapshotter := fakeSnapshotter{data: []byte("ok")}
	frame := buildSnapshotResponseFrame(nil, cameraLookup.GetCamera, snapshotter.Snapshot)
	body, ok := frame.Body.(*syncpb.StationMessage_CameraSnapshotResponse)
	require.True(t, ok)
	require.Equal(t, "camera name is required", body.CameraSnapshotResponse.Error)
}

func TestBuildSnapshotResponseFrame_CameraNotFound(t *testing.T) {
	cameraLookup := fakeCameraLookup{ok: false}
	snapshotter := fakeSnapshotter{}
	frame := buildSnapshotResponseFrame(&syncpb.CameraSnapshotRequest{CameraName: "missing"}, cameraLookup.GetCamera, snapshotter.Snapshot)
	body, ok := frame.Body.(*syncpb.StationMessage_CameraSnapshotResponse)
	require.True(t, ok)
	require.Equal(t, "camera not found", body.CameraSnapshotResponse.Error)
}

func TestBuildSnapshotResponseFrame_SnapshotError(t *testing.T) {
	cameraLookup := fakeCameraLookup{ok: true}
	snapshotter := fakeSnapshotter{err: errors.New("snap failed")}
	frame := buildSnapshotResponseFrame(
		&syncpb.CameraSnapshotRequest{CameraName: "cam1"},
		cameraLookup.GetCamera,
		snapshotter.Snapshot,
	)
	body, ok := frame.Body.(*syncpb.StationMessage_CameraSnapshotResponse)
	require.True(t, ok)
	require.Equal(t, "snap failed", body.CameraSnapshotResponse.Error)
}

func TestBuildSnapshotResponseFrame_Success(t *testing.T) {
	cameraLookup := fakeCameraLookup{ok: true}
	snapshotter := fakeSnapshotter{data: []byte("img")}
	frame := buildSnapshotResponseFrame(
		&syncpb.CameraSnapshotRequest{CameraName: "cam1"},
		cameraLookup.GetCamera,
		snapshotter.Snapshot,
	)
	body, ok := frame.Body.(*syncpb.StationMessage_CameraSnapshotResponse)
	require.True(t, ok)
	require.Equal(t, []byte("img"), body.CameraSnapshotResponse.Data)
	require.Equal(t, "", body.CameraSnapshotResponse.Error)
}
