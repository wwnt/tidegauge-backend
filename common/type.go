package common

import (
	"encoding/json"
	"github.com/google/uuid"
	"tide/pkg/custype"
)

type Status = string
type MsgType = uint8
type StringMsecMap = map[string]custype.TimeMillisecond
type StringMapMap = map[string]map[string]string // map[deviceName]map[itemType]itemName
type UUIDStringsMap = map[uuid.UUID][]string

type StationInfoStruct struct {
	Identifier string       `json:"identifier"`
	Devices    StringMapMap `json:"devices"`
	Cameras    []string     `json:"cameras"`
}

type SendMsgStruct struct {
	Type MsgType `json:"type"`
	Body any     `json:"body"`
}

type ReceiveMsgStruct struct {
	Type MsgType         `json:"type"`
	Body json.RawMessage `json:"body"`
}

type RpiStatusStruct struct {
	CpuTemp float64 `json:"cpu_temp"`
}

type RpiStatusTimeStruct struct {
	RpiStatusStruct
	Millisecond custype.TimeMillisecond `json:"msec"`
}

type DataTimeStruct struct {
	Value       float64                 `json:"val"`
	Millisecond custype.TimeMillisecond `json:"msec"`
}

type ItemNameDataTimeStruct struct {
	ItemName string `json:"item_name"`
	DataTimeStruct
}

type PortTerminalStruct struct {
	DeviceName string `json:"device_name"`
	Command    string `json:"command"`
}

type StatusChangeStruct struct {
	Status    Status                  `json:"status"`
	ChangedAt custype.TimeMillisecond `json:"changed_at"`
}

// StationStatusStruct is used for station status change msg
type StationStatusStruct struct {
	StationId  uuid.UUID `json:"station_id"`
	Identifier string    `json:"identifier"`
	StatusChangeStruct
}

// ItemStatusStruct is used for item status change msg
type ItemStatusStruct struct {
	ItemName string `json:"item_name"`
	StatusChangeStruct
}

type StationIdItemStatusStruct struct {
	StationId uuid.UUID `json:"station_id"`
	ItemStatusStruct
}

type RowIdItemStatusStruct struct {
	RowId int64 `json:"row_id"`
	ItemStatusStruct
}

type FullItemStatusStruct struct {
	StationId  uuid.UUID `json:"station_id"`
	Identifier string    `json:"identifier"`
	RowIdItemStatusStruct
}

type StationItemStruct struct {
	StationId uuid.UUID `json:"station_id"`
	ItemName  string    `json:"item_name"`
}
