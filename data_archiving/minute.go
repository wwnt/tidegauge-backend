package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"time"
)

type DataValue struct {
	Value float64
	Time  time.Time
}

// 生成1min实时数据文件,
func generateMinuteDataFile(minuteDataMap map[string][]float64, start time.Time, stationId string) error {
	// water_temp, salinity, tide_level, air_temp, pressure, humidity, rainfall, wind_data
	var fileTime = start
	for i := 0; i < 24*60; i++ {
		var content string
		content += fmt.Sprintf("DT %s\r\n", fileTime.Format("20060102150405"))

		if val, ok := minuteDataMap["water_temperature"]; ok {
			if len(val) > i && !math.IsNaN(val[i]) {
				content += fmt.Sprintf("WT %5.1f\r\n", val[i])
			}
		}
		if val, ok := minuteDataMap["water_salinity"]; ok {
			if len(val) > i && !math.IsNaN(val[i]) {
				content += fmt.Sprintf("SL %4.1f\r\n", val[i])
			}
		}
		if val, ok := minuteDataMap["water_level_shaft"]; ok {
			if len(val) > i && !math.IsNaN(val[i]) {
				content += fmt.Sprintf("WL %4.f\r\n", val[i]*100)
			}
		}
		content += fmt.Sprintf("DT %s\r\n", fileTime.Format("20060102150405"))

		if val, ok := minuteDataMap["air_temperature"]; ok {
			if len(val) > i && !math.IsNaN(val[i]) {
				content += fmt.Sprintf("AT %5.1f\r\n", val[i])
			}
		}
		if val, ok := minuteDataMap["air_pressure"]; ok {
			if len(val) > i && !math.IsNaN(val[i]) {
				content += fmt.Sprintf("BP %6.1f\r\n", val[i])
			}
		}
		if val, ok := minuteDataMap["air_humidity"]; ok {
			if len(val) > i && !math.IsNaN(val[i]) {
				content += fmt.Sprintf("HU %3.f\r\n", val[i])
			}
		}
		if val, ok := minuteDataMap["rainfall"]; ok {
			if len(val) > i && !math.IsNaN(val[i]) {
				content += fmt.Sprintf("RN %7.1f\r\n", val[i])
			}
		}
		if val, ok := minuteDataMap["wind_speed"]; ok {
			if len(val) > i && !math.IsNaN(val[i]) {
				content += fmt.Sprintf("WS %4.1f %3.f\r\n", val[i], minuteDataMap["wind_direction"][i])
			}
		}
		if val, ok := minuteDataMap["air_visibility"]; ok {
			if len(val) > i && !math.IsNaN(val[i]) {
				content += fmt.Sprintf("VB %4.1f", val[i]/1000)
			}
		}
		filename := fmt.Sprintf("SQ%s.%s", fileTime.Format("200601021504"), stationId)
		err := os.WriteFile(filename, []byte(content), 0644)
		if err != nil {
			log.Printf("Failed to generate one minute file: %s, err: %s", filename, err)
			return err
		}
		fileTime = fileTime.Add(time.Minute)
	}
	return nil
}

// 生成正点报文数据文件
func generateHourlyReportDataFile(minuteDataMap map[string][]float64, start time.Time, stationId string, stationNum string) error {
	var hourDataMap = make(map[string][]float64)
	for dataType, minuteData := range minuteDataMap {
		var hourData []float64
		nextHour := 0
		for i, datum := range minuteData {
			if math.IsNaN(datum) {
				continue
			}
			dataHour := i / 60
		ReCmp:
			if dataHour == nextHour {
				hourData = append(hourData, datum)
				nextHour++
			} else if dataHour > nextHour {
				hourData = append(hourData, math.NaN())
				nextHour++
				goto ReCmp
			}
			if nextHour >= 24 {
				break
			}
		}
		hourDataMap[dataType] = hourData
	}
	bjLocation, _ := time.LoadLocation("Asia/Shanghai")
	fileTime := start.Add(time.Hour * 6)
	for count := 0; count < 4; count++ {
		content := fmt.Sprintf("ZCZC 896\r\n")
		content += fmt.Sprintf("(OHM) %sZ %s\r\nAAXX\r\n", stationId, fileTime.Add(24*time.Hour).Format("0215"))
		for hour := 0; hour < 6; hour++ {
			idx := hour + count*6
			dataTime := fileTime.Add(-time.Duration(5-hour) * time.Hour)

			var airTempSign = 0
			if hourDataMap["air_temperature"][idx] < 0 {
				airTempSign = 1
			}
			var airPressure = hourDataMap["air_pressure"][idx]
			if airPressure > 1000 {
				airPressure = (airPressure - 1000) * 10
			} else {
				airPressure = airPressure * 10
			}
			content += fmt.Sprintf("%s1 %s %1d3/%2d /%02.f%02.f 1%1d%03.f 4%04.f\r\n",
				dataTime.Format("0215"), stationNum, 3, findVisibilityCode(hourDataMap["air_visibility"][idx]/1000),
				hourDataMap["wind_direction"][idx]/10, hourDataMap["wind_speed"][idx], airTempSign, math.Abs(hourDataMap["air_temperature"][idx]*10),
				airPressure)

			var waterTempSign = 0
			if hourDataMap["water_temperature"][idx] < 0 {
				waterTempSign = 1
			}
			content += fmt.Sprintf("22200 0%1d%3.f\n", waterTempSign, math.Abs(hourDataMap["water_temperature"][idx]*10))

			content += fmt.Sprintf("555//\r\n")
			var waterLevel = hourDataMap["water_level_shaft"][idx] * 100
			if waterLevel < 0 {
				waterLevel = waterLevel + 500
			} else if waterLevel > 1000 {
				waterLevel = waterLevel - 1000
			}

			if waterLevel < 1000 {
				content += fmt.Sprintf("883%s%03.f =\r\n", dataTime.In(bjLocation).Format("02 15"), waterLevel)
			}
			//for _, tidal := range tidalDataValue {
			//	if tidal.Time.Hour() == dataTime.Hour() {
			//
			//		break
			//	}
			//}
		}
		content += fmt.Sprintf("\r\nNNNN")
		// YYYYMMDDHH+<.>+SSS
		fileName := fmt.Sprintf("%s.%s", fileTime.Format("010215"), stationId)
		err := os.WriteFile(fileName, []byte(content), 0644)
		if err != nil {
			return err
		}
		fileTime = fileTime.Add(time.Hour * 6)
	}

	return nil
}
