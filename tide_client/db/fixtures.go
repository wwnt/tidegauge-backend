package db

import (
	"fmt"

	"tide/common"
)

// 本文件是 db 和 controller 两个包的测试共用的固定数据。它不能放进 _test.go
// （那样 controller 的测试引用不到），所以特意不依赖 testing 包，免得把测试框架
// 链接进正式程序。

var (
	StatusLogs = []common.RowIdItemStatusStruct{
		{
			RowId: 1,
			ItemStatusStruct: common.ItemStatusStruct{
				ItemName:           "item1",
				StatusChangeStruct: common.StatusChangeStruct{Status: common.Normal, ChangedAt: 1100},
			},
		},
		{
			RowId: 2,
			ItemStatusStruct: common.ItemStatusStruct{
				ItemName:           "item2",
				StatusChangeStruct: common.StatusChangeStruct{Status: common.Abnormal, ChangedAt: 1200},
			},
		},
	}
	DataHis = []common.ItemNameDataTimeStruct{
		{
			ItemName:       "item1",
			DataTimeStruct: common.DataTimeStruct{Value: 1, Millisecond: 1100},
		},
		{
			ItemName:       "item2",
			DataTimeStruct: common.DataTimeStruct{Value: 1, Millisecond: 1100},
		},
	}
)

// ResetFixtures 清空测试库，并重新写入 StatusLogs 和 DataHis。
func ResetFixtures() error {
	_, err := db.Exec(`
delete from item_status_log where true;
drop table if exists item1;
drop table if exists item2;
`)
	if err != nil {
		return err
	}

	for _, statusLog := range StatusLogs {
		rowId, err := SaveItemStatusLog(statusLog.ItemName, statusLog.Status, statusLog.ChangedAt.ToInt64())
		if err != nil {
			return err
		}
		if rowId != statusLog.RowId {
			return fmt.Errorf("status log fixture %q got row id %d, want %d", statusLog.ItemName, rowId, statusLog.RowId)
		}
	}
	for _, data := range DataHis {
		if err = MakeSureTableExist(data.ItemName); err != nil {
			return err
		}
		if err = SaveData(data.ItemName, data.Value, data.Millisecond.ToInt64()); err != nil {
			return err
		}
	}
	return nil
}
