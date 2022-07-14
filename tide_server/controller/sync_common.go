package controller

import (
	"encoding/json"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"io"
	"sync"
	"tide/common"
	"tide/pkg/custype"
	"tide/pkg/pubsub"
	"tide/tide_server/auth"
	"tide/tide_server/db"
	"time"
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
	kMsgUpdateStationStatus   = "update_station_status"
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

func Publish(pub *pubsub.PubSub, msg interface{}, key interface{}) {
	if err := pub.Publish(msg, key); err != nil {
		zap.L().WithOptions(zap.AddCallerSkip(1)).DPanic("publish", zap.Error(err))
	}
}

func sendToConfigPubSub(typ string, body interface{}) {
	if err := configPubSub.Publish(SendMsgStruct{typ, body}, nil); err != nil {
		zap.L().WithOptions(zap.AddCallerSkip(1)).DPanic("publish", zap.Error(err))
	}
}

type SendMsgStruct struct {
	Type string      `json:"type"`
	Body interface{} `json:"body"`
}
type RcvMsgStruct struct {
	Type string          `json:"type"`
	Body json.RawMessage `json:"body"`
}

func UpdateStationStatus(pub *pubsub.PubSub, stationId uuid.UUID, identifier string, status common.Status) (ok bool) {
	now := custype.ToTimeMillisecond(time.Now())
	if n, err := db.UpdateStationStatus(stationId, status, now.ToTime()); err != nil {
		zap.L().Error("db", zap.Error(err))
		return
	} else if n > 0 {
		Publish(pub, SendMsgStruct{Type: kMsgUpdateStationStatus,
			Body: common.StationStatusStruct{
				StationId:          stationId,
				Identifier:         identifier,
				StatusChangeStruct: common.StatusChangeStruct{Status: status, ChangedAt: now},
			}}, nil)
	}
	return true
}

const (
	connTypeWebBrowser = 1 << iota
	connTypeSyncData
	connTypeSyncConfig
	connTypeAny = -1
)

// connections with downstream or websocket. Used to send data
var usersConns = struct {
	sync.Mutex
	m map[string]map[pubsub.PubConn]int
}{
	m: make(map[string]map[pubsub.PubConn]int),
}

func addUserConn(username string, conn pubsub.PubConn, typ int) {
	usersConns.Lock()
	defer usersConns.Unlock()
	if conns, ok := usersConns.m[username]; ok {
		conns[conn] = typ
	} else {
		usersConns.m[username] = map[pubsub.PubConn]int{conn: typ}
	}
}

func delUserConn(username string, conn pubsub.PubConn) {
	usersConns.Lock()
	defer usersConns.Unlock()
	delete(usersConns.m[username], conn)
	if len(usersConns.m[username]) == 0 {
		delete(usersConns.m, username)
	}
}

func closeConnByUser(username string, typ int) {
	usersConns.Lock()
	defer usersConns.Unlock()
	conns, ok := usersConns.m[username]
	if ok {
		for conn, t := range conns {
			if typ&t != 0 {
				_ = conn.Close()
			}
		}
	}
}

func closeAllConn(typ int) {
	usersConns.Lock()
	defer usersConns.Unlock()
	for username, conns := range usersConns.m {
		delete(usersConns.m, username)
		for conn, t := range conns {
			if typ&t != 0 {
				_ = conn.Close()
			}
		}
	}
}

func changeUserPermissionScope(username string, permissions map[uuid.UUID][]string) {
	usersConns.Lock()
	defer usersConns.Unlock()
	conns := usersConns.m[username]
	if len(conns) > 0 {
		var permTopic pubsub.TopicMap
		if permissions != nil {
			permTopic = uuidStringsMapToTopic(permissions)
		}
		var tmpSyncConns []io.WriteCloser
		for conn, t := range conns {
			if connTypeWebBrowser&t != 0 {
				dataPubSub.LimitTopicScope(conn, permTopic)
			}
			if connTypeSyncData&t != 0 {
				_ = conn.Close()
			}
			if connTypeSyncConfig&t != 0 {
				tmpSyncConns = append(tmpSyncConns, conn)
			}
		}
		if len(tmpSyncConns) > 0 {
			localAvail, err := db.GetAvailableItems()
			if err != nil {
				logger.Error(err.Error())
				for _, conn := range tmpSyncConns {
					_ = conn.Close()
				}
			}
			var downstreamAvail = make(common.UUIDStringsMap)
			for _, stationItem := range localAvail {
				if _, ok := permTopic[stationItem]; ok || permTopic == nil {
					downstreamAvail[stationItem.StationId] = append(downstreamAvail[stationItem.StationId], stationItem.ItemName)
				}
			}
			for _, conn := range tmpSyncConns {
				err = json.NewEncoder(conn).Encode(SendMsgStruct{Type: kMsgUpdateAvailable, Body: downstreamAvail})
				if err != nil {
					_ = conn.Close()
				}
			}
		}
	}
}

func changeUserAvailableScope(localAvail map[uuid.UUID][]string) {
	usersConns.Lock()
	defer usersConns.Unlock()
	for username, conns := range usersConns.m {
		if len(conns) > 0 {
			var syncConfigConns []io.WriteCloser
			for conn, t := range conns {
				if connTypeSyncConfig&t != 0 {
					syncConfigConns = append(syncConfigConns, conn)
				}
			}
			if len(syncConfigConns) > 0 {
				user, err := userManager.GetUser(username)
				if err != nil {
					logger.Error(err.Error())
					for _, conn := range syncConfigConns {
						_ = conn.Close()
					}
					continue
				}
				var permTopic pubsub.TopicMap
				if user.Role == auth.NormalUser {
					permissions, err := authorization.GetPermissions(username)
					if err != nil {
						logger.Error(err.Error())
						for _, conn := range syncConfigConns {
							_ = conn.Close()
						}
						continue
					}
					permTopic = uuidStringsMapToTopic(permissions)
				}
				var downstreamAvail = make(common.UUIDStringsMap)
				for stationId, items := range localAvail {
					for _, itemName := range items {
						if _, ok := permTopic[common.StationItemStruct{StationId: stationId, ItemName: itemName}]; ok || permTopic == nil {
							downstreamAvail[stationId] = append(downstreamAvail[stationId], itemName)
						}
					}
				}
				for _, conn := range syncConfigConns {
					err = json.NewEncoder(conn).Encode(SendMsgStruct{Type: kMsgUpdateAvailable, Body: downstreamAvail})
					if err != nil {
						_ = conn.Close()
					}
				}
			}
		}
	}
}
