package device

import (
	"log/slog"
	"maps"
	"os"
	"strings"
)

// getStringInBetween Returns empty string if no start string found
func getStringInBetween(str, start, end string) string {
	_, rest, ok := strings.Cut(str, start)
	if !ok {
		return ""
	}
	between, _, ok := strings.Cut(rest, end)
	if !ok {
		return ""
	}
	return between
}

// MergeInfo 把设备的 item 信息合并进站点信息表，校验失败直接终止进程。
//
// 注意：本函数与 AddCronJob/AddCronJobWithOneItem 一样是**启动期** API——所有调用点
// 都在 controller.addDevices 装配设备的过程中。若未来需要支持"运行期改配置不重启"，
// 这些函数要先改为返回 error，当前调用即退出（fail-fast）的语义不适用于运行期。
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
			// 必须拷贝：srcItems 往往就是驱动自己的 conf.Items，驱动的 cron 任务会遍历它。
			// 直接存引用的话，之后同名 device 合并进来的其他驱动的 item 会混进这张表，
			// 该驱动每个周期都会为不属于自己的 item 上报一次"缺失读数"。
			dstInfo[deviceName] = maps.Clone(srcItems)
		}
	}
}
