package device

import (
	"log/slog"
	"os"
	"strings"
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
		slog.Error("srcInfo length is 0")
		os.Exit(1)
	}
	for deviceName, srcItems := range srcInfo {
		if deviceName == "" {
			slog.Error("device_name is empty")
			os.Exit(1)
		}
		if len(srcItems) == 0 {
			slog.Error("srcItems length is 0")
			os.Exit(1)
		}
		if dstItems, ok := dstInfo[deviceName]; ok {
			for itemType, itemName := range srcItems {
				if itemType == "" {
					slog.Error("item_type is empty")
					os.Exit(1)
				}
				if itemName == "" {
					slog.Error("item_name is empty")
					os.Exit(1)
				}
				if _, ok := dstItems[itemType]; ok {
					slog.Error(itemType + " duplicate")
					os.Exit(1)
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
