package syncv2

import (
	"maps"
	"tide/common"

	syncpb "tide/pkg/pb/syncproto"
)

func StationInfoToPB(info common.StationInfoStruct) *syncpb.StationInfo {
	ret := &syncpb.StationInfo{
		Identifier: info.Identifier,
		Devices:    make(map[string]*syncpb.DeviceItems, len(info.Devices)),
		Cameras:    append([]string(nil), info.Cameras...),
	}
	for deviceName, items := range info.Devices {
		di := &syncpb.DeviceItems{Items: make(map[string]string, len(items))}
		maps.Copy(di.Items, items)
		ret.Devices[deviceName] = di
	}
	return ret
}

func PBToStationInfo(info *syncpb.StationInfo) common.StationInfoStruct {
	ret := common.StationInfoStruct{
		Identifier: info.Identifier,
		Devices:    make(common.StringMapMap),
		Cameras:    append([]string(nil), info.Cameras...),
	}
	for deviceName, items := range info.Devices {
		ret.Devices[deviceName] = make(map[string]string)
		maps.Copy(ret.Devices[deviceName], items.Items)
	}
	return ret
}
