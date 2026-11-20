package syncv2relay

import (
	"encoding/json"
	"fmt"

	"tide/common"
	"tide/pkg/custype"
	syncpb "tide/pkg/pb/syncproto"
	"tide/pkg/pubsub"
	"tide/tide_server/db"

	"github.com/google/uuid"
)

// Message types kept as strings for wire compatibility with existing sync consumers.
const (
	MsgSyncStation           = "SyncStation"
	MsgSyncStationCannotEdit = "SyncStationCannotEdit"
	MsgDelUpstreamStation    = "DelUpstreamStation"
	MsgSyncDevice            = "SyncDevice"
	MsgDelDevice             = "DelDevice"
	MsgSyncItem              = "SyncItem"
	MsgDelItem               = "DelItem"
	MsgEditDeviceRecord      = "EditDeviceRecord"
	MsgUpdateAvailable       = "update_available"
	MsgUpdateStationStatus   = "UpdateStationStatus"
	MsgMissItemStatus        = "MissItemStatus"
	MsgUpdateItemStatus      = "UpdateItemStatus"
	MsgMissData              = "MissData"
	MsgData                  = "data"
	MsgDataGpio              = "data_gpio"
)

func uuidStringsMapToTopics(permissions common.UUIDStringsMap) pubsub.TopicSet {
	permissionTopics := make(pubsub.TopicSet)
	for sid, items := range permissions {
		for _, item := range items {
			permissionTopics[common.StationItemStruct{StationId: sid, ItemName: item}] = struct{}{}
		}
	}
	return permissionTopics
}

func dbStationFullToProto(sf db.StationFullInfo) *syncpb.RelayStationFull {
	pb := &syncpb.RelayStationFull{
		Id:                sf.Id.String(),
		Identifier:        sf.Identifier,
		Name:              sf.Name,
		IpAddr:            sf.IpAddr,
		Location:          sf.Location,
		Partner:           sf.Partner,
		Cameras:           sf.Cameras,
		Status:            sf.Status,
		StatusChangedAtMs: sf.StatusChangedAt.ToInt64(),
		Upstream:          sf.Upstream,
	}

	for _, d := range sf.Devices {
		pb.Devices = append(pb.Devices, &syncpb.RelayDevice{
			StationId:         d.StationId.String(),
			Name:              d.Name,
			Specs:             d.Specs,
			LastMaintenanceMs: d.LastMaintenance.ToInt64(),
		})
	}
	for _, i := range sf.Items {
		pb.Items = append(pb.Items, &syncpb.RelayItem{
			StationId:         i.StationId.String(),
			Name:              i.Name,
			Type:              i.Type,
			DeviceName:        i.DeviceName,
			Status:            i.Status,
			StatusChangedAtMs: i.StatusChangedAt.ToInt64(),
			Available:         i.Available,
		})
	}

	return pb
}

func dbDeviceRecordToProto(dr db.DeviceRecord) *syncpb.RelayDeviceRecord {
	return &syncpb.RelayDeviceRecord{
		Id:          dr.Id.String(),
		StationId:   dr.StationId.String(),
		DeviceName:  dr.DeviceName,
		Config:      dr.Record,
		CreatedAtMs: dr.CreatedAt.ToInt64(),
		UpdatedAtMs: dr.UpdatedAt.ToInt64(),
		Version:     int32(dr.Version),
	}
}

func protoToDbStation(pb *syncpb.RelayStationFull) db.Station {
	id, _ := uuid.Parse(pb.Id)
	return db.Station{
		Id:              id,
		Identifier:      pb.Identifier,
		Name:            pb.Name,
		IpAddr:          pb.IpAddr,
		Location:        pb.Location,
		Partner:         pb.Partner,
		Cameras:         pb.Cameras,
		Status:          pb.Status,
		StatusChangedAt: custype.UnixMs(pb.StatusChangedAtMs),
		Upstream:        pb.Upstream,
	}
}

// jsonNull 是 JSON 的 null。devices.specs 是 NOT NULL 的 jsonb 列（默认值就是 JSON null），
// 而 pgx 会把 nil 的 json.RawMessage 编码成 SQL NULL。
var jsonNull = json.RawMessage("null")

func protoToDbDevice(pb *syncpb.RelayDevice, stationID uuid.UUID) db.Device {
	if stationID == uuid.Nil {
		stationID, _ = uuid.Parse(pb.StationId)
	}
	specs := json.RawMessage(pb.Specs)
	if len(specs) == 0 {
		// 对端没带 specs（旧版本上游，或字段本来就是空）：写入 JSON null，不能写 SQL NULL。
		specs = jsonNull
	}
	return db.Device{
		StationId:       stationID,
		Name:            pb.Name,
		Specs:           specs,
		LastMaintenance: custype.UnixMs(pb.LastMaintenanceMs),
	}
}

func protoToDbItem(pb *syncpb.RelayItem) db.Item {
	stationID, _ := uuid.Parse(pb.StationId)
	return db.Item{
		StationId:       stationID,
		Name:            pb.Name,
		Type:            pb.Type,
		DeviceName:      pb.DeviceName,
		Status:          pb.Status,
		StatusChangedAt: custype.UnixMs(pb.StatusChangedAtMs),
		Available:       pb.Available,
	}
}

func protoToDbDeviceRecord(pb *syncpb.RelayDeviceRecord) db.DeviceRecord {
	id, _ := uuid.Parse(pb.Id)
	stationID, _ := uuid.Parse(pb.StationId)
	return db.DeviceRecord{
		Id:         id,
		StationId:  stationID,
		DeviceName: pb.DeviceName,
		Record:     pb.Config,
		CreatedAt:  custype.UnixMs(pb.CreatedAtMs),
		UpdatedAt:  custype.UnixMs(pb.UpdatedAtMs),
		Version:    int(pb.Version),
	}
}

func applyAvailableItems(cfg DownstreamConfig, deps DownstreamDeps, avail *syncpb.RelayAvailableItems) error {
	body := make(common.UUIDStringsMap)
	for stationIDStr, itemList := range avail.Stations {
		stationID, err := uuid.Parse(stationIDStr)
		if err != nil {
			continue
		}
		body[stationID] = itemList.ItemNames
	}

	changed, err := deps.Store.UpdateAvailableItems(cfg.UpstreamID, body)
	if err != nil {
		return err
	}
	if changed {
		deps.Notifier.BroadcastAvailableChange(body)
	}
	return nil
}

func applyFullConfig(cfg DownstreamConfig, deps DownstreamDeps, batch *syncpb.RelayConfigBatch) error {
	oldStations, err := deps.Store.GetStationsByUpstreamID(cfg.UpstreamID)
	if err != nil {
		return err
	}

	newStationSet := make(map[uuid.UUID]struct{})
	for _, pbStation := range batch.Stations {
		stationID, parseErr := uuid.Parse(pbStation.Id)
		if parseErr != nil {
			continue
		}
		newStationSet[stationID] = struct{}{}

		station := protoToDbStation(pbStation)
		oldDevices, _ := deps.Store.GetDevices(stationID)
		oldItems, _ := deps.Store.GetItems(stationID)

		if changed, syncErr := deps.Store.SyncStation(cfg.UpstreamID, station); syncErr != nil {
			return syncErr
		} else if changed {
			deps.Notifier.PublishConfig(MsgSyncStation, station)
		}

		if changed, syncErr := deps.Store.SyncStationCannotEdit(stationID, station.Cameras); syncErr != nil {
			return syncErr
		} else if changed {
			deps.Notifier.PublishConfig(MsgSyncStationCannotEdit, station)
		}

		statusChangedAt := custype.UnixMs(pbStation.StatusChangedAtMs)
		if changed, syncErr := deps.Store.UpdateStationStatus(stationID, pbStation.Status, statusChangedAt.ToTime()); syncErr != nil {
			return syncErr
		} else if changed {
			deps.Notifier.PublishConfig(MsgUpdateStationStatus, common.StationStatusStruct{
				StationId:          stationID,
				Identifier:         pbStation.Identifier,
				StatusChangeStruct: common.StatusChangeStruct{Status: pbStation.Status, ChangedAt: custype.UnixMs(pbStation.StatusChangedAtMs)},
			})
		}

		newDeviceSet := make(map[string]struct{})
		for _, pbDev := range pbStation.Devices {
			devStationID, _ := uuid.Parse(pbDev.StationId)
			device := protoToDbDevice(pbDev, devStationID)
			newDeviceSet[devStationID.String()+pbDev.Name] = struct{}{}
			if changed, syncErr := deps.Store.SyncDevice(device); syncErr != nil {
				return syncErr
			} else if changed {
				deps.Notifier.PublishConfig(MsgSyncDevice, device)
			}
		}
		for _, d := range oldDevices {
			if _, ok := newDeviceSet[d.StationId.String()+d.Name]; !ok {
				if changed, delErr := deps.Store.DelDevice(d.StationId, d.Name); delErr != nil {
					return delErr
				} else if changed {
					deps.Notifier.PublishConfig(MsgDelDevice, d)
				}
			}
		}

		newItemSet := make(map[common.StationItemStruct]struct{})
		for _, pbItem := range pbStation.Items {
			itemStationID, _ := uuid.Parse(pbItem.StationId)
			item := protoToDbItem(pbItem)
			newItemSet[common.StationItemStruct{StationId: itemStationID, ItemName: pbItem.Name}] = struct{}{}
			if changed, syncErr := deps.Store.SyncItem(item); syncErr != nil {
				return syncErr
			} else if changed {
				deps.Notifier.PublishConfig(MsgSyncItem, item)
			}
		}
		for _, item := range oldItems {
			if _, ok := newItemSet[common.StationItemStruct{StationId: item.StationId, ItemName: item.Name}]; !ok {
				if changed, delErr := deps.Store.DelItem(item.StationId, item.Name); delErr != nil {
					return delErr
				} else if changed {
					deps.Notifier.PublishConfig(MsgDelItem, item)
				}
			}
		}
	}

	for _, station := range oldStations {
		if _, ok := newStationSet[station.Id]; !ok {
			if changed, delErr := deps.Store.DelUpstreamStation(cfg.UpstreamID, station.Id); delErr != nil {
				return delErr
			} else if changed {
				deps.Notifier.PublishConfig(MsgDelUpstreamStation, station.Id)
			}
		}
	}

	for _, pbDR := range batch.DeviceRecords {
		dr := protoToDbDeviceRecord(pbDR)
		if changed, syncErr := deps.Store.SyncDeviceRecord(dr); syncErr != nil {
			return syncErr
		} else if changed {
			deps.Notifier.PublishConfig(MsgEditDeviceRecord, dr)
		}
	}

	return nil
}

func applyConfigEvent(cfg DownstreamConfig, deps DownstreamDeps, evt *syncpb.RelayConfigEvent) error {
	if evt == nil {
		return nil
	}
	if deps.EditLock != nil {
		deps.EditLock.Lock()
		defer deps.EditLock.Unlock()
	}

	var (
		changed bool
		err     error
	)

	switch evt.Type {
	case MsgSyncStation:
		var station db.Station
		if err = json.Unmarshal(evt.Payload, &station); err != nil {
			return err
		}
		changed, err = deps.Store.SyncStation(cfg.UpstreamID, station)
	case MsgSyncStationCannotEdit:
		var station db.Station
		if err = json.Unmarshal(evt.Payload, &station); err != nil {
			return err
		}
		changed, err = deps.Store.SyncStationCannotEdit(station.Id, station.Cameras)
	case MsgDelUpstreamStation:
		var stationID uuid.UUID
		if err = json.Unmarshal(evt.Payload, &stationID); err != nil {
			return err
		}
		changed, err = deps.Store.DelUpstreamStation(cfg.UpstreamID, stationID)
	case MsgSyncDevice:
		var device db.Device
		if err = json.Unmarshal(evt.Payload, &device); err != nil {
			return err
		}
		changed, err = deps.Store.SyncDevice(device)
	case MsgDelDevice:
		var device db.Device
		if err = json.Unmarshal(evt.Payload, &device); err != nil {
			return err
		}
		changed, err = deps.Store.DelDevice(device.StationId, device.Name)
	case MsgSyncItem:
		var item db.Item
		if err = json.Unmarshal(evt.Payload, &item); err != nil {
			return err
		}
		changed, err = deps.Store.SyncItem(item)
	case MsgDelItem:
		var item db.Item
		if err = json.Unmarshal(evt.Payload, &item); err != nil {
			return err
		}
		changed, err = deps.Store.DelItem(item.StationId, item.Name)
	case MsgEditDeviceRecord:
		var dr db.DeviceRecord
		if err = json.Unmarshal(evt.Payload, &dr); err != nil {
			return err
		}
		changed, err = deps.Store.SyncDeviceRecord(dr)
	case MsgUpdateStationStatus:
		var body common.StationStatusStruct
		if err = json.Unmarshal(evt.Payload, &body); err != nil {
			return err
		}
		changed, err = deps.Store.UpdateStationStatus(body.StationId, body.Status, body.ChangedAt.ToTime())
		if err != nil {
			return err
		}
		if changed {
			deps.Notifier.PublishStatus(evt.Type, json.RawMessage(evt.Payload))
		}
		return nil
	case MsgMissItemStatus, MsgUpdateItemStatus:
		var body common.FullItemStatusStruct
		if err = json.Unmarshal(evt.Payload, &body); err != nil {
			return err
		}
		inserted, err := deps.Store.UpdateAndSaveStatusLog(body.StationId, body.RowId, body.ItemName, body.Status, body.ChangedAt.ToTime())
		if err != nil {
			return err
		}
		if inserted {
			deps.Notifier.PublishStatus(evt.Type, json.RawMessage(evt.Payload))
		}
		return nil
	case MsgUpdateAvailable:
		var body common.UUIDStringsMap
		if err = json.Unmarshal(evt.Payload, &body); err != nil {
			return err
		}
		changed, err = deps.Store.UpdateAvailableItems(cfg.UpstreamID, body)
		if err != nil {
			return err
		}
		if changed {
			deps.Notifier.BroadcastAvailableChange(body)
		}
		return nil
	default:
		return nil
	}

	if err != nil {
		return err
	}
	if changed {
		deps.Notifier.PublishConfig(evt.Type, json.RawMessage(evt.Payload))
	}
	return nil
}

// applyStatusEvent 落库一条增量状态事件。返回的错误表示落库失败：调用方应结束会话，
// 让下一次连接按水位（max(row_id)）把这条事件重放回来——吞掉错误继续处理的话，
// 后续事件会把水位推过去，这条状态变化就永久丢了。
func applyStatusEvent(deps DownstreamDeps, evt *syncpb.RelayStatusEvent) error {
	if evt == nil {
		return nil
	}
	stationID, err := uuid.Parse(evt.StationId)
	if err != nil {
		return nil // 无法识别的帧，跳过
	}

	if evt.ItemName == "" {
		evtChangedAt := custype.UnixMs(evt.ChangedAtUnixMs)
		changed, err := deps.Store.UpdateStationStatus(stationID, evt.Status, evtChangedAt.ToTime())
		if err != nil {
			return fmt.Errorf("update station status: %w", err)
		}
		if changed {
			deps.Notifier.PublishStatus(MsgUpdateStationStatus, common.StationStatusStruct{
				StationId:          stationID,
				Identifier:         evt.Identifier,
				StatusChangeStruct: common.StatusChangeStruct{Status: evt.Status, ChangedAt: custype.UnixMs(evt.ChangedAtUnixMs)},
			})
		}
		return nil
	}

	statusLog := common.RowIdItemStatusStruct{
		RowId: evt.RowId,
		ItemStatusStruct: common.ItemStatusStruct{
			ItemName: evt.ItemName,
			StatusChangeStruct: common.StatusChangeStruct{
				Status:    evt.Status,
				ChangedAt: custype.UnixMs(evt.ChangedAtUnixMs),
			},
		},
	}
	inserted, err := deps.Store.UpdateAndSaveStatusLog(stationID, statusLog.RowId, statusLog.ItemName, statusLog.Status, statusLog.ChangedAt.ToTime())
	if err != nil {
		return fmt.Errorf("save item status log: %w", err)
	}
	if inserted {
		deps.Notifier.PublishStatus(MsgUpdateItemStatus, common.FullItemStatusStruct{
			StationId:             stationID,
			Identifier:            evt.Identifier,
			RowIdItemStatusStruct: statusLog,
		})
	}
	return nil
}

// applyDataBatch 落库一批数据点。返回的错误表示落库失败，处理方式同 applyStatusEvent：
// 结束会话，由下一次连接按每个 item 的 max(timestamp) 水位重放。
func applyDataBatch(deps DownstreamDeps, batch *syncpb.RelayDataBatch) error {
	if batch == nil || batch.StationId == "" || len(batch.Points) == 0 {
		return nil
	}
	stationID, err := uuid.Parse(batch.StationId)
	if err != nil {
		return nil // 无法识别的帧，跳过
	}

	for _, point := range batch.Points {
		if err = deps.Store.EnsureDataTable(point.ItemName); err != nil {
			return fmt.Errorf("ensure data table %q: %w", point.ItemName, err)
		}
		tm := custype.UnixMs(point.UnixMs)
		inserted, saveErr := deps.Store.SaveDataHistory(stationID, point.ItemName, point.Value, tm.ToTime())
		if saveErr != nil {
			return fmt.Errorf("save data point for %q: %w", point.ItemName, saveErr)
		}
		if !inserted {
			continue
		}

		stationItem := common.StationItemStruct{StationId: stationID, ItemName: point.ItemName}
		if point.Kind == syncpb.DataKind_DATA_KIND_GPIO {
			_, _ = deps.Store.UpdateItemStatus(stationID, point.ItemName, common.NoStatus, tm.ToTime())
		}

		data := common.DataTimeStruct{Value: point.Value, Millisecond: tm}
		if batch.DataType == MsgMissData {
			deps.Notifier.PublishMissData(stationItem, data)
			continue
		}
		deps.Notifier.PublishData(stationItem, data, point.Kind == syncpb.DataKind_DATA_KIND_GPIO)
	}
	return nil
}
