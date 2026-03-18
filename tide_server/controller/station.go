package controller

import (
	"log/slog"
	"net/http"
	"strconv"

	"tide/pkg/custype"
	"tide/tide_server/auth"
	"tide/tide_server/db"

	"github.com/google/uuid"
	"github.com/hashicorp/yamux"
)

func ListStation(w http.ResponseWriter, r *http.Request) {
	stations, err := db.GetStations()
	if err != nil {
		slog.Error("Failed to get stations list", "error", err)
		return
	}
	writeJSON(w, http.StatusOK, stations)
}

func EditStation(w http.ResponseWriter, r *http.Request) {
	var station db.Station
	if !readJSONOrBadRequest(w, r, &station) {
		slog.Error("Failed to bind station data")
		return
	}
	editMu.Lock()
	defer editMu.Unlock()
	if err := db.EditStation(&station); err != nil {
		slog.Error("Failed to edit station", "error", err)
		return
	}
	if !station.Upstream { // Only sync the local station
		hub.Publish(BrokerConfig, SendMsgStruct{Type: kMsgSyncStation, Body: station}, nil)
	}
	writeOK(w)
}

func DelStation(w http.ResponseWriter, r *http.Request) {
	editMu.Lock()
	defer editMu.Unlock()

	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	stationId, err := uuid.Parse(r.Form.Get("id"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Can only delete local stations
	if n, err := db.DelLocalStation(stationId); err != nil {
		slog.Error("Failed to delete local station", "station_id", stationId, "error", err)
		return
	} else if n > 0 {
		if value, ok := recvConnections.Load(stationId); ok {
			_ = value.(*yamux.Session).Close()
		}
		hub.Publish(BrokerConfig, SendMsgStruct{Type: kMsgDelUpstreamStation, Body: stationId}, nil)
	}
	writeOK(w)
}

func ListDevice(w http.ResponseWriter, r *http.Request) {
	stationId, _ := uuid.Parse(r.URL.Query().Get("station_id"))
	devices, err := db.GetDevices(stationId)
	if err != nil {
		slog.Error("Failed to get devices", "station_id", stationId, "error", err)
		return
	}
	writeJSON(w, http.StatusOK, devices)
}

func EditDevice(w http.ResponseWriter, r *http.Request) {
	editMu.Lock()
	defer editMu.Unlock()

	var device db.Device
	if !readJSONOrBadRequest(w, r, &device) {
		slog.Error("Failed to bind device data")
		return
	}
	// Can only edit local stations
	if up, err := db.IsUpstreamStation(device.StationId); err != nil || up {
		return
	}
	if err := db.EditDevice(device); err != nil {
		slog.Error("Failed to edit device", "device", device.Name, "error", err)
		return
	}
	hub.Publish(BrokerConfig, SendMsgStruct{Type: kMsgSyncDevice, Body: device}, nil)
	writeOK(w)
}

func ListDeviceRecord(w http.ResponseWriter, r *http.Request) {
	deviceRecords, err := db.GetDeviceRecords()
	if err != nil {
		slog.Error("Failed to get device records", "error", err)
		return
	}
	writeJSON(w, http.StatusOK, deviceRecords)
}

func EditDeviceRecord(w http.ResponseWriter, r *http.Request) {
	var dr db.DeviceRecord
	if !readJSONOrBadRequest(w, r, &dr) {
		slog.Error("Failed to bind device record data")
		return
	}
	if dr.Id == uuid.Nil {
		if up, err := db.IsUpstreamStation(dr.StationId); err != nil || up {
			return
		}
	}

	editMu.Lock()
	defer editMu.Unlock()
	if err := db.EditDeviceRecord(&dr); err != nil {
		slog.Error("Failed to edit device record", "device_record_id", dr.Id, "error", err)
		return
	}
	hub.Publish(BrokerConfig, SendMsgStruct{Type: kMsgEditDeviceRecord, Body: dr}, nil)
	writeOK(w)
}

func ListItem(w http.ResponseWriter, r *http.Request) {
	var stationId uuid.UUID
	var err error
	if raw := r.URL.Query().Get("station_id"); raw != "" {
		stationId, err = uuid.Parse(raw)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}

	items, err := db.GetItems(stationId)
	if err != nil {
		slog.Error("Failed to get items", "station_id", stationId, "error", err)
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func DataHistory(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	itemName := q.Get("item_name")
	if itemName == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	stationId, err := uuid.Parse(q.Get("station_id"))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	start, _ := strconv.ParseInt(q.Get("start"), 10, 64)
	end, _ := strconv.ParseInt(q.Get("end"), 10, 64)

	if requestRole(r) < auth.Admin {
		if !authorization.CheckPermission(requestUsername(r), stationId, itemName) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
	}

	ds, err := db.GetDataHistory(stationId, itemName, custype.UnixMs(start), custype.UnixMs(end))
	if err != nil {
		slog.Error("Failed to get data history", "station_id", stationId, "item_name", itemName, "error", err)
		return
	}
	writeJSON(w, http.StatusOK, ds)
}

//func bind(c *gin.Context, obj any) bool {
//	if errs := c.ShouldBind(obj); errs != nil {
//		logger.Error(errs.Error())
//		if errs, ok := errs.(validator.ValidationErrors); ok {
//			for _, err := range errs {
//				_, _ = c.Writer.WriteString(err.Field() + ": input error\n")
//			}
//		}
//		return false
//	} else {
//		return true
//	}
//}
