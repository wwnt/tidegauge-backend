package controller

import (
	"encoding/json"
	"github.com/google/uuid"
	"go.uber.org/zap"
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

func Publish(pub *pubsub.PubSub, msg any, key any) {
	if err := pub.Publish(msg, key); err != nil {
		zap.L().WithOptions(zap.AddCallerSkip(1)).DPanic("publish", zap.Error(err))
	}
}

func sendToConfigPubSub(typ string, body any) {
	if err := configPubSub.Publish(SendMsgStruct{typ, body}, nil); err != nil {
		zap.L().WithOptions(zap.AddCallerSkip(1)).DPanic("publish", zap.Error(err))
	}
}

type SendMsgStruct struct {
	Type string `json:"type"`
	Body any    `json:"body"`
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
	m map[string]map[pubsub.Subscriber]int
}{
	m: make(map[string]map[pubsub.Subscriber]int),
}

func addUserConn(username string, conn pubsub.Subscriber, typ int) {
	usersConns.Lock()
	defer usersConns.Unlock()
	if conns, ok := usersConns.m[username]; ok {
		conns[conn] = typ
	} else {
		usersConns.m[username] = map[pubsub.Subscriber]int{conn: typ}
	}
}

func delUserConn(username string, conn pubsub.Subscriber) {
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
	subscribers, ok := usersConns.m[username]
	if ok {
		for subscriber, t := range subscribers {
			if typ&t != 0 {
				subscriber <- nil
			}
		}
	}
}

func closeAllConn(typ int) {
	usersConns.Lock()
	defer usersConns.Unlock()
	for username, subscribers := range usersConns.m {
		delete(usersConns.m, username)
		for subscriber, t := range subscribers {
			if typ&t != 0 {
				subscriber <- nil
			}
		}
	}
}

func handlePermissionChange(username string, permissions map[uuid.UUID][]string) {
	usersConns.Lock()
	defer usersConns.Unlock()

	var permTopic pubsub.TopicMap
	if permissions != nil {
		permTopic = uuidStringsMapToTopic(permissions)
	}
	var syncConfigConns []pubsub.Subscriber
	for subscriber, t := range usersConns.m[username] {
		if connTypeWebBrowser&t != 0 {
			dataPubSub.LimitTopicScope(subscriber, permTopic)
		}
		if connTypeSyncData&t != 0 {
			subscriber <- nil
		}
		if connTypeSyncConfig&t != 0 {
			syncConfigConns = append(syncConfigConns, subscriber)
		}
	}
	if len(syncConfigConns) > 0 {
		localAvail, err := db.GetAvailableItems()
		if err != nil {
			logger.Error(err.Error())
			for _, subscriber := range syncConfigConns {
				subscriber <- nil
			}
			return
		}
		var downstreamAvail = make(common.UUIDStringsMap)
		for _, stationItem := range localAvail {
			if _, ok := permTopic[stationItem]; ok || permTopic == nil {
				downstreamAvail[stationItem.StationId] = append(downstreamAvail[stationItem.StationId], stationItem.ItemName)
			}
		}
		j, err := json.Marshal(SendMsgStruct{Type: kMsgUpdateAvailable, Body: downstreamAvail})
		if err != nil {
			return
		}
		for _, subscriber := range syncConfigConns {
			select {
			case subscriber <- j:
			default:
				subscriber <- nil
			}
		}
	}
}

func handleAvailableChange(localAvail map[uuid.UUID][]string) {
	usersConns.Lock()
	defer usersConns.Unlock()
	for username, subscribers := range usersConns.m {
		var syncConfigSubscribers []pubsub.Subscriber
		for subscriber, t := range subscribers {
			if connTypeSyncConfig&t != 0 {
				syncConfigSubscribers = append(syncConfigSubscribers, subscriber)
			}
		}
		if len(syncConfigSubscribers) > 0 {
			user, err := userManager.GetUser(username)
			if err != nil {
				logger.Error(err.Error())
				for _, subscriber := range syncConfigSubscribers {
					subscriber <- nil
				}
				continue
			}
			var permTopic pubsub.TopicMap
			if user.Role == auth.NormalUser {
				permissions, err := authorization.GetPermissions(username)
				if err != nil {
					logger.Error(err.Error())
					for _, subscriber := range syncConfigSubscribers {
						subscriber <- nil
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
			j, err := json.Marshal(SendMsgStruct{Type: kMsgUpdateAvailable, Body: downstreamAvail})
			if err != nil {
				return
			}
			for _, subscriber := range syncConfigSubscribers {
				select {
				case subscriber <- j:
				default:
					subscriber <- nil
				}
			}
		}
	}
}

func handleAddItems() {
	localAvail, err := db.GetAvailableItems()
	if err != nil {
		logger.Error(err.Error())
		return
	}
	usersConns.Lock()
	defer usersConns.Unlock()
	for username, subscribers := range usersConns.m {
		var syncConfigSubscribers []pubsub.Subscriber
		for subscriber, t := range subscribers {
			if connTypeSyncConfig&t != 0 {
				syncConfigSubscribers = append(syncConfigSubscribers, subscriber)
			}
		}
		if len(syncConfigSubscribers) > 0 {
			user, err := userManager.GetUser(username)
			if err != nil {
				logger.Error(err.Error())
				for _, subscriber := range syncConfigSubscribers {
					subscriber <- nil
				}
				continue
			}
			var permTopic pubsub.TopicMap
			if user.Role == auth.NormalUser {
				permissions, err := authorization.GetPermissions(username)
				if err != nil {
					logger.Error(err.Error())
					for _, subscriber := range syncConfigSubscribers {
						subscriber <- nil
					}
					continue
				}
				permTopic = uuidStringsMapToTopic(permissions)
			}
			var downstreamAvail = make(common.UUIDStringsMap)
			for _, stationItem := range localAvail {
				if _, ok := permTopic[stationItem]; ok || permTopic == nil {
					downstreamAvail[stationItem.StationId] = append(downstreamAvail[stationItem.StationId], stationItem.ItemName)
				}
			}
			j, err := json.Marshal(SendMsgStruct{Type: kMsgUpdateAvailable, Body: downstreamAvail})
			if err != nil {
				return
			}
			for _, subscriber := range syncConfigSubscribers {
				select {
				case subscriber <- j:
				default:
					subscriber <- nil
				}
			}
		}
	}
}
