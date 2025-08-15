package main

import (
	"cmp"
	"fmt"
	"log"
	"os"
	"time"
)

// 生成整点实时数据文件（以潮高为例，其他类似）
func generateHourlyDataFile(oneDayDataMap map[string][]DataValue, start time.Time, stationId string) error {
	// 文件名: <WT>+MMDD+<.>+IIIII
	// 表层水温数据文件只有一行记录,包括:日期、24个整点值和两个极值,内容如下:
	// YYYYMMDD+空格+00点测值+空格+01点测值+空格+……+23点测值+空格+日最高值+空格+日最低值+回车换行
	oneDayData, ok := oneDayDataMap["water_temperature"]
	if ok {
		var content string
		content += fmt.Sprintf("%s", start.Format("20060102"))
		var minVal, maxVal = oneDayData[0].Value, oneDayData[0].Value
		nextHour := 0
		for _, dataValue := range oneDayData {
			minVal = min(minVal, dataValue.Value)
			maxVal = max(maxVal, dataValue.Value)

			if dataValue.Time.Hour() == nextHour {
				content += fmt.Sprintf(" %5.1f", dataValue.Value)
				nextHour++
			} else if dataValue.Time.Hour() > nextHour {
				content += fmt.Sprintf(" %5.1f", 999.)
				nextHour++
			}
		}
		content += fmt.Sprintf(" %5.1f %5.1f\r\n", maxVal, minVal)
		fileName := fmt.Sprintf("wt%s.%s", start.Format("0102"), stationId)
		err := os.WriteFile(fileName, []byte(content), 0644)
		if err != nil {
			return err
		}
	}
	// 文件名: <SL>+MMDD+<.>+IIIII
	// 表层盐度数据文件只有一行记录,包括:日期、24个整点值和两个极值,内容如下:
	// YYYYMMDD+空格+00点测值+空格+01点测值+空格+……+23点测值+空格+日最高值+空格+日最低值+回车换行
	oneDayData, ok = oneDayDataMap["water_salinity"]
	if ok {
		var content string
		content += fmt.Sprintf("%s", start.Format("20060102"))
		var minVal, maxVal = oneDayData[0].Value, oneDayData[0].Value
		nextHour := 0
		for _, dataValue := range oneDayData {
			minVal = min(minVal, dataValue.Value)
			maxVal = max(maxVal, dataValue.Value)

			if dataValue.Time.Hour() == nextHour {
				content += fmt.Sprintf(" %6.1f", dataValue.Value)
				nextHour++
			} else if dataValue.Time.Hour() > nextHour {
				content += fmt.Sprintf(" %6.1f", 9999.)
				nextHour++
			}
		}
		content += fmt.Sprintf(" %6.1f %6.1f\r\n", maxVal, minVal)
		fileName := fmt.Sprintf("sl%s.%s", start.Format("0102"), stationId)
		err := os.WriteFile(fileName, []byte(content), 0644)
		if err != nil {
			return err
		}
	}
	// 文件名: <WL>+MMDD+<_DAT.>IIIII
	// 逐时潮高整点数据文件只有一行记录,包括:日期、24个整点值和极值,内容如下:
	// YYYMMDD+空格+00点测值+空格+01点测值+空格+……+23点测值+空格+高/低潮潮高+空格 + 高/低 潮 潮 时 (HHMI)+ 空 格 + 高/低潮潮高 + 空 格 + 高/低 潮 潮 时 (HHMI)+ 空格+……+回车换行
	oneDayData, ok = oneDayDataMap["water_level_shaft"]
	if ok {
		var content string
		content += fmt.Sprintf("%s", start.Format("20060102"))
		var minVal, maxVal = oneDayData[0].Value * 100, oneDayData[0].Value * 100
		var minTime, maxTime = oneDayData[0].Time, oneDayData[0].Time
		nextHour := 0
		for _, dataValue := range oneDayData {
			dataValue.Value = dataValue.Value * 100
			if cmp.Compare(minVal, dataValue.Value) > 0 {
				minVal = dataValue.Value
				minTime = dataValue.Time
			}
			if cmp.Compare(maxVal, dataValue.Value) < 0 {
				maxVal = dataValue.Value
				maxTime = dataValue.Time
			}
			if dataValue.Time.Hour() == nextHour {
				content += fmt.Sprintf(" %4.f", dataValue.Value)
				nextHour++
			} else if dataValue.Time.Hour() > nextHour {
				content += fmt.Sprintf(" %4.f", 9999.)
				nextHour++
			}
		}
		content += fmt.Sprintf(" %4.f %s %4.f %s %4.f %s %4.f %s %4.f %s %4.f %s\r\n",
			maxVal, maxTime.Format("1504"),
			9999., "    ",
			9999., "    ",
			minVal, minTime.Format("1504"),
			9999., "    ",
			9999., "    ",
		)
		fileName := fmt.Sprintf("wl%s_dat.%s", start.Format("0102"), stationId)
		err := os.WriteFile(fileName, []byte(content), 0644)
		if err != nil {
			return err
		}
	}
	// 文件名: <WL>+MMDD+<.>+IIIII
	// 1min潮高整点数据文件由26行记录组成,内容如下:
	// YYYYMMDD+回车换行
	// <00H:>+空格 +00时00分测值 + 空格 +00时01分测值 + 空格 +00时02分测值 + 空格+……+00时59分测值+回车换行
	// <01H:>+空格 +01时00分测值 + 空格 +01时01分测值 + 空格 +01时02分测值 + 空格+……+01时59分测值+回车换行
	// …………
	// <23H:>+空格 +23时00分测值 + 空格 +23时01分测值 + 空格 +23时02分测值 + 空格+……+23时59分测值+回车换行
	// 高/低潮潮高 + 空格 + 高/低潮潮时(HHMI)+ 空格 + 高/低 潮 潮 高 + 空 格 + 高/低潮潮时(HHMI)+空格+……+回车换行
	oneDayData, ok = oneDayDataMap["water_level_shaft"]
	if ok {
		var content string
		content += fmt.Sprintf("%s\r\n", start.Format("20060102"))
		var minVal, maxVal = oneDayData[0].Value * 100, oneDayData[0].Value * 100
		var minTime, maxTime = oneDayData[0].Time, oneDayData[0].Time
		nextHour := 0
		nextMinute := 0
		for _, dataValue := range oneDayData {
			dataValue.Value = dataValue.Value * 100
			if cmp.Compare(minVal, dataValue.Value) > 0 {
				minVal = dataValue.Value
				minTime = dataValue.Time
			}
			if cmp.Compare(maxVal, dataValue.Value) < 0 {
				maxVal = dataValue.Value
				maxTime = dataValue.Time
			}
			if dataValue.Time.Hour() == nextHour {
				if nextMinute == 0 {
					content += fmt.Sprintf("%02d:", nextHour)
				}

				if dataValue.Time.Minute() == nextMinute {
					content += fmt.Sprintf(" %4.f", dataValue.Value)
					nextMinute++
				} else if dataValue.Time.Minute() > nextMinute {
					content += fmt.Sprintf(" %4.f", 9999.)
					nextMinute++
				}

				if nextMinute == 60 {
					content += "\r\n"
					nextHour++
					nextMinute = 0
				}
			} else if dataValue.Time.Hour() > nextHour {
				nextHour++
			}
		}
		content += fmt.Sprintf(" %4.f %s %4.f %s %4.f %s %4.f %s %4.f %s %4.f %s\r\n",
			maxVal, maxTime.Format("1504"),
			9999., "    ",
			9999., "    ",
			minVal, minTime.Format("1504"),
			9999., "    ",
			9999., "    ",
		)
		fileName := fmt.Sprintf("wl%s.%s", start.Format("0102"), stationId)
		err := os.WriteFile(fileName, []byte(content), 0644)
		if err != nil {
			return err
		}
	}
	// 文件名: <AT>+MMDD+<.>+IIIII
	// 气温数据文件只有一行记录,包括:日期、24个整点值和两个极值,内容如下:
	// YYYYMMDD+空格+21点测值+空格+22点测值+空格+23点测值+空格+00点测值+空格+01点测值+……+空格+20点测值+空格+日最高值+空格+日最低值+回车换行
	oneDayData, ok = oneDayDataMap["air_temperature"]
	if ok {
		var content string
		content += fmt.Sprintf("%s", start.Format("20060102"))
		var minVal, maxVal = oneDayData[0].Value, oneDayData[0].Value
		nextHour := 0
		for _, dataValue := range oneDayData {
			minVal = min(minVal, dataValue.Value)
			maxVal = max(maxVal, dataValue.Value)

			if dataValue.Time.Hour() == nextHour {
				content += fmt.Sprintf(" %5.1f", dataValue.Value)
				nextHour++
			} else if dataValue.Time.Hour() > nextHour {
				content += fmt.Sprintf(" %5.1f", 999.)
				nextHour++
			}
		}
		content += fmt.Sprintf(" %5.1f %5.1f\r\n", maxVal, minVal)
		fileName := fmt.Sprintf("at%s.%s", start.Format("0102"), stationId)
		err := os.WriteFile(fileName, []byte(content), 0644)
		if err != nil {
			return err
		}
	}
	// 文件名: <BP>+MMDD+<.>+IIIII
	// 气压数据文件只有一行记录,包括:日期、24个整点值和两个极值,内容如下:
	// YYYYMMDD+空格+21点测值+空格+22点测值+空格+23点测值+空格+00点测值+空格+01点测值+……+空格+20点测值+空格+日最高值+空格+日最低值+回车换行
	oneDayData, ok = oneDayDataMap["air_pressure"]
	if ok {
		var content string
		content += fmt.Sprintf("%s", start.Format("20060102"))
		var minVal, maxVal = oneDayData[0].Value, oneDayData[0].Value
		nextHour := 0
		for _, dataValue := range oneDayData {
			minVal = min(minVal, dataValue.Value)
			maxVal = max(maxVal, dataValue.Value)

			if dataValue.Time.Hour() == nextHour {
				content += fmt.Sprintf(" %6.1f", dataValue.Value)
				nextHour++
			} else if dataValue.Time.Hour() > nextHour {
				content += fmt.Sprintf(" %6.1f", 9999.)
				nextHour++
			}
		}
		content += fmt.Sprintf(" %6.1f %6.1f\r\n", maxVal, minVal)
		fileName := fmt.Sprintf("bp%s.%s", start.Format("0102"), stationId)
		err := os.WriteFile(fileName, []byte(content), 0644)
		if err != nil {
			return err
		}
	}
	// 文件名: <RN>+MMDD+<.>+IIIII
	// 降水量数据文件只有一行记录,包括:日期、24个整点值和两个极值,内容如下:
	//YYYYMMDD+空格+21点测值+空格+22点测值+空格+23点测值+空格+00点测值+空
	//格+01点测值+……+空格+20点测值+空格+20-08时降水量+空格+08-20时降水量+空格+
	//日降水总量+回车换行
	oneDayData, ok = oneDayDataMap["rainfall"]
	if ok {
		var content string
		content += fmt.Sprintf("%s", start.Format("20060102"))
		nextHour := 0
		var acc [2]float64
		for _, dataValue := range oneDayData {
			acc[dataValue.Time.Hour()/12] += dataValue.Value
			if dataValue.Time.Hour() == nextHour {
				content += fmt.Sprintf(" %7.1f", dataValue.Value)
				nextHour++
			} else if dataValue.Time.Hour() > nextHour {
				content += fmt.Sprintf(" %7.1f", 999.)
				nextHour++
			}
		}
		content += fmt.Sprintf(" %7.1f %7.1f\r\n", acc[0], acc[1])
		fileName := fmt.Sprintf("rn%s.%s", start.Format("0102"), stationId)
		err := os.WriteFile(fileName, []byte(content), 0644)
		if err != nil {
			return err
		}
	}
	// 文件名: <VB>+MMDD+<.>+IIIII
	// 能见度数据文件只有一行记录,包括:日期、24个整点值和两个极值,内容如下:
	//YYYYMMDD+空格+21点测值+空格+22点测值+空格+23点测值+空格+00点测值+空
	//格+01点测值+……+空格+20点测值+空格+日最高值+空格+日最低值+回车换行
	oneDayData, ok = oneDayDataMap["air_visibility"]
	if ok {
		var content string
		content += fmt.Sprintf("%s", start.Format("20060102"))
		var minVal, maxVal = oneDayData[0].Value / 1000, oneDayData[0].Value / 1000
		nextHour := 0
		for _, dataValue := range oneDayData {
			dataValue.Value /= 1000
			minVal = min(minVal, dataValue.Value)
			maxVal = max(maxVal, dataValue.Value)

			if dataValue.Time.Hour() == nextHour {
				content += fmt.Sprintf(" %4.1f", dataValue.Value)
				nextHour++
			} else if dataValue.Time.Hour() > nextHour {
				content += fmt.Sprintf(" %4.1f", 99.)
				nextHour++
			}
		}
		content += fmt.Sprintf(" %4.1f %4.1f\r\n", maxVal, minVal)
		fileName := fmt.Sprintf("vb%s.%s", start.Format("0102"), stationId)
		err := os.WriteFile(fileName, []byte(content), 0644)
		if err != nil {
			return err
		}
	}
	// 文件名: <HU>+MMDD+<.>+IIIII
	// 相对湿度数据文件只有一行记录,包括:日期、24个整点值和两个极值,内容如下:
	//YYYYMMDD+空格+21点测值+空格+22点测值+空格+23点测值+空格+00点测值+空
	//格+01点测值+……+空格+20点测值+空格+日最高值+空格+日最低值+回车换行
	oneDayData, ok = oneDayDataMap["air_humidity"]
	if ok {
		var content string
		content += fmt.Sprintf("%s", start.Format("20060102"))
		var minVal, maxVal = oneDayData[0].Value, oneDayData[0].Value
		nextHour := 0
		for _, dataValue := range oneDayData {
			minVal = min(minVal, dataValue.Value)
			maxVal = max(maxVal, dataValue.Value)

			if dataValue.Time.Hour() == nextHour {
				content += fmt.Sprintf(" %5.1f", dataValue.Value)
				nextHour++
			} else if dataValue.Time.Hour() > nextHour {
				content += fmt.Sprintf(" %5.1f", 999.)
				nextHour++
			}
		}
		content += fmt.Sprintf(" %5.1f %5.1f\r\n", maxVal, minVal)
		fileName := fmt.Sprintf("hu%s.%s", start.Format("0102"), stationId)
		err := os.WriteFile(fileName, []byte(content), 0644)
		if err != nil {
			return err
		}
	}
	// 文件名: <WS>+MMDD+<_DAT.>+IIIII
	// 逐时风速风向整点数据文件由5行数据记录组成,内容如下:
	// YYYYMMDD+空格+21点风向测值+空格+21点风速测值+空格+22点风向测值+空格+22点风速测值+空格+23点风向测值+空格+23点风速测值+空格+00点风向测值+空格+00点风向测值+
	//空格+01点风向测值+空格+01点风速测值+……+空格+20点风向测值+空格+20点风速测值+回车换行
	// 20-23时极大风对应的风向+空格+20-23时极大风速+空格+23-02时极大风对应的风向+空格+23-02时极大风速+02-05时极大风对应的风向+空格+02-05时极大风速+空格+05-08时极大风对应的风向 +
	//空格+05-08时极大风速+空格 + 08-11时极大风对应的风向 + 空格+08-11时极大风速+空格+11-14时极大风对应的风向+空格+11-14时极大风速+
	//空格+14-17时极大风对应的风向+空格+14-17时极大风速+空格+17-20时极大风对应的风向+空格+17-20时极大风速+回车换行
	// 最大风速+空格+风向+空格+出现的时间(HHMI)+回车换行
	// 极大风速+空格+风向+空格+出现的时间(HHMI)+回车换行
	// 大于17m/s风速出现的起止时间1(HHMI.HHMI)+空格+……+大于17m/s风速出现的起止时间18(HHMI.HHMI)+回车换行
	oneDayData, ok = oneDayDataMap["wind_speed"]
	windDirOneDayData := oneDayDataMap["wind_direction"]
	if ok && len(windDirOneDayData) == len(oneDayData) {
		var content string
		content += fmt.Sprintf("%s", start.Format("20060102"))
		var maxValIdx int
		var maxVal float64
		var eachThereHourMaxIdxs []int
		var eachThereHourMaxIdx int
		var eachThereHourMax float64
		nextHour := 0
		nextThereHour := 3
		for i, dataValue := range oneDayData {
			if cmp.Compare(maxVal, dataValue.Value) < 0 {
				maxVal = dataValue.Value
				maxValIdx = i
			}

			if dataValue.Time.Hour() == nextThereHour {
				eachThereHourMaxIdxs = append(eachThereHourMaxIdxs, eachThereHourMaxIdx)
				eachThereHourMax = 0
				nextThereHour += 3
			}
			if cmp.Compare(eachThereHourMax, dataValue.Value) < 0 {
				eachThereHourMax = dataValue.Value
				eachThereHourMaxIdx = i
			}

			if dataValue.Time.Hour() == nextHour {
				content += fmt.Sprintf(" %3.f %4.1f", windDirOneDayData[i].Value, dataValue.Value)
				nextHour++
			} else if dataValue.Time.Hour() > nextHour {
				content += fmt.Sprintf(" %3.f %4.1f", 999., 99.)
				nextHour++
			}
		}
		content += "\r\n"
		for _, idx := range eachThereHourMaxIdxs {
			content += fmt.Sprintf("%3.f %4.1f ", windDirOneDayData[idx].Value, oneDayData[idx].Value)
		}
		content = content[:len(content)-1]
		content += "\r\n"
		content += fmt.Sprintf("%4.1f %3.f %s\r\n", oneDayData[maxValIdx].Value, windDirOneDayData[maxValIdx].Value, oneDayData[maxValIdx].Time.Format("1504"))
		content += fmt.Sprintf("%4.1f %3.f %s\r\n", oneDayData[maxValIdx].Value, windDirOneDayData[maxValIdx].Value, oneDayData[maxValIdx].Time.Format("1504"))
		fileName := fmt.Sprintf("ws%s_dat.%s", start.Format("0102"), stationId)
		err := os.WriteFile(fileName, []byte(content), 0644)
		if err != nil {
			return err
		}
	} else {
		log.Printf("Failed to generate data file: %v", "wind_speed")
	}
	// 文件名: <WS>+MMDD+<.>+IIIII
	// 10min风速风向整点数据文件由29行数据记录组成,内容如下:
	oneDayData, ok = oneDayDataMap["wind_speed"]
	if ok && len(windDirOneDayData) == len(oneDayData) {
		var content string
		content += fmt.Sprintf("%s\r\n", start.Format("20060102"))
		var maxValIdx int
		var maxVal float64
		var eachThereHourMaxIdxs []int
		var eachThereHourMaxIdx int
		var eachThereHourMax float64
		nextHour := 0
		nextMinute := 0
		nextThereHour := 3
		for i, dataValue := range oneDayData {
			if cmp.Compare(maxVal, dataValue.Value) < 0 {
				maxVal = dataValue.Value
				maxValIdx = i
			}

			if dataValue.Time.Hour() == nextThereHour {
				eachThereHourMaxIdxs = append(eachThereHourMaxIdxs, eachThereHourMaxIdx)
				eachThereHourMax = 0
				nextThereHour += 3
			}
			if cmp.Compare(eachThereHourMax, dataValue.Value) < 0 {
				eachThereHourMax = dataValue.Value
				eachThereHourMaxIdx = i
			}

			if dataValue.Time.Hour() == nextHour {
				if nextMinute == 0 {
					content += fmt.Sprintf("%02d:", nextHour)
				}

				if dataValue.Time.Minute() == nextMinute {
					content += fmt.Sprintf(" %3.f %4.1f", windDirOneDayData[i].Value, dataValue.Value)
					nextMinute += 10
				} else if dataValue.Time.Minute() > nextMinute {
					content += fmt.Sprintf(" %3.f %4.1f", 999., 99.)
					nextMinute += 10
				}

				if nextMinute == 60 {
					content += "\r\n"
					nextHour++
					nextMinute = 0
				}
			} else if dataValue.Time.Hour() > nextHour {
				for nextMinute = 0; nextMinute < 60; nextMinute += 10 {
					content += fmt.Sprintf(" %3.f %4.1f", 999., 99.)
				}
				content += "\r\n"
				nextHour++
				nextMinute = 0
			}
		}
		for _, idx := range eachThereHourMaxIdxs {
			content += fmt.Sprintf("%3.f %4.1f ", windDirOneDayData[idx].Value, oneDayData[idx].Value)
		}
		content = content[:len(content)-1]
		content += "\r\n"
		content += fmt.Sprintf("%4.1f %3.f %s\r\n", oneDayData[maxValIdx].Value, windDirOneDayData[maxValIdx].Value, oneDayData[maxValIdx].Time.Format("1504"))
		content += fmt.Sprintf("%4.1f %3.f %s\r\n", oneDayData[maxValIdx].Value, windDirOneDayData[maxValIdx].Value, oneDayData[maxValIdx].Time.Format("1504"))
		fileName := fmt.Sprintf("ws%s.%s", start.Format("0102"), stationId)
		err := os.WriteFile(fileName, []byte(content), 0644)
		if err != nil {
			return err
		}
	} else {
		log.Printf("Failed to generate data file: %v", "wind_speed")
	}
	return nil
}
