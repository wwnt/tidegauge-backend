package device

import (
	"strings"
	"tide/tide_client/global"
)

// getStringInBetween Returns empty string if no start string found
func getStringInBetween(str string, start string, end string) (result string) {
	s := strings.Index(str, start)
	if s == -1 {
		return
	}
	newStr := str[s+len(start):]
	e := strings.Index(newStr, end)
	if e == -1 {
		return
	}
	return newStr[:e]
}

func MergeInfo(dstInfo map[string]map[string]string, srcInfo map[string]map[string]string) {
	if len(srcInfo) == 0 {
		global.Log.Fatal("srcInfo length is 0")
	}
	for deviceName, srcItems := range srcInfo {
		if deviceName == "" {
			global.Log.Fatal("device_name is empty")
		}
		if len(srcItems) == 0 {
			global.Log.Fatal("srcItems length is 0")
		}
		if dstItems, ok := dstInfo[deviceName]; ok {
			for itemType, itemName := range srcItems {
				if itemType == "" {
					global.Log.Fatal("item_type is empty")
				}
				if itemName == "" {
					global.Log.Fatal("item_name is empty")
				}
				if _, ok := dstItems[itemType]; ok {
					global.Log.Fatal(itemType + " duplicate")
				} else {
					dstItems[itemType] = itemName
				}
			}
		} else {
			dstInfo[deviceName] = srcItems
		}
	}
}

// Float64P returns a pointer of a float64 variable
func Float64P(value float64) *float64 {
	return &value
}
