package main

import (
	"database/sql"
	"fmt"
	"log"
	"math"
	"os"
	"path"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type MinuteData struct {
	Timestamp time.Time
	WaterTemp float64
	Salinity  float64
	TideLevel float64
	AirTemp   float64
	Pressure  float64
	Humidity  float64
	Rainfall  string
	WindData  string
}

var dataTypeMap = map[string]string{
	"rain_intensity":               "drd11a_analog_out",
	"air_pressure":                 "location1_air_pressure",
	"water_conductivity":           "location1_water_conductivity",
	"water_level":                  "location1_water_level_pls_c",
	"water_salinity":               "location1_water_salinity",
	"water_temperature":            "location1_water_temperature",
	"water_total_dissolved_solids": "location1_water_total_dissolved_solids",
	"rainfall":                     "rain_volume",
	"water_level_shaft":            "location1_water_level_shaft",
	"air_temperature":              "location1_air_temperature",
	"air_humidity":                 "location1_air_humidity",
	"air_visibility":               "location1_air_visibility",
	"radar_water_distance":         "location1_radar_water_distance",
	"wind_speed":                   "location1_wind_speed",
	"wind_direction":               "location1_wind_direction",
}

// 从数据库获取数据并存储到Map中
func getDataFromDB(db *sql.DB, tableName string, start, end time.Time) ([]DataValue, error) {
	rows, err := db.Query("SELECT"+" value, timestamp FROM "+tableName+" WHERE station_id = $3 and timestamp >= $1 AND timestamp < $2 ORDER BY timestamp;", start, end, "fbeaabd4-70f6-11ef-a2cb-000c2916f493")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var oneDayData []DataValue
	for rows.Next() {
		var dataValue DataValue
		err := rows.Scan(&dataValue.Value, &dataValue.Time)
		if err != nil {
			return nil, err
		}
		dataValue.Time = dataValue.Time.UTC()
		oneDayData = append(oneDayData, dataValue)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return oneDayData, nil
}
func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	// 建立数据库连接
	db, err := sql.Open("pgx", fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		"192.168.1.6", 5432, "postgres", "wwnt$pg", "tidegauge"))
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer func() { _ = db.Close() }()

	day := 1
DO:
	yesterday := time.Now().UTC().Add(-24 * time.Hour * time.Duration(day))
	startOfDay := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, time.UTC)
	endOfDay := startOfDay.Add(24 * time.Hour)

	var oneDayDataMap = make(map[string][]DataValue)
	// 先取出所有数据
	for dataType, tableName := range dataTypeMap {
		oneDayData, err := getDataFromDB(db, tableName, startOfDay, endOfDay)
		if err != nil {
			log.Println(err)
			return
		}
		oneDayDataMap[dataType] = oneDayData
	}

	var minuteDataMap = make(map[string][]float64)
	for dataType, oneDayData := range oneDayDataMap {
		var minuteData []float64
		nextMinute := 0
		for _, datum := range oneDayData {
			dataMinute := datum.Time.Hour()*60 + datum.Time.Minute()
		ReCmp:
			if dataMinute == nextMinute {
				minuteData = append(minuteData, datum.Value)
				nextMinute++
			} else if dataMinute > nextMinute {
				minuteData = append(minuteData, math.NaN())
				nextMinute++
				goto ReCmp
			}
		}
		minuteDataMap[dataType] = minuteData
	}

	rootDir := path.Join("E:/tidegauge", startOfDay.Format("20060102"))
	var wkDir string
	// 生成1分钟数据文件
	wkDir = path.Join(rootDir, "1-海洋站分钟数据")
	if err = os.MkdirAll(wkDir, os.ModePerm); err != nil {
		log.Fatalf("Failed to create directory: %v", err)
	}
	if err = os.Chdir(wkDir); err != nil {
		log.Fatalf("Failed to change directory: %v", err)
	}
	err = generateMinuteDataFile(minuteDataMap, startOfDay, "07499")
	if err != nil {
		log.Printf("Failed to generate data file: %v", err)
		return
	}

	// 生成整点数据文件
	wkDir = path.Join(rootDir, "2-海洋站整点数据")
	if err = os.MkdirAll(wkDir, os.ModePerm); err != nil {
		log.Fatalf("Failed to create directory: %v", err)
	}
	if err = os.Chdir(wkDir); err != nil {
		log.Fatalf("Failed to change directory: %v", err)
	}
	err = generateHourlyDataFile(oneDayDataMap, startOfDay, "07499")
	if err != nil {
		log.Printf("Failed to generate data file: %v", err)
		return
	}

	// 生成正点数据文件
	wkDir = path.Join(rootDir, "3-海洋站正点数据")
	if err = os.MkdirAll(wkDir, os.ModePerm); err != nil {
		log.Fatalf("Failed to create directory: %v", err)
	}
	if err = os.Chdir(wkDir); err != nil {
		log.Fatalf("Failed to change directory: %v", err)
	}
	err = generateHourlyReportDataFile(minuteDataMap, startOfDay, "BLG", "07428")
	if err != nil {
		log.Printf("Failed to generate data file: %v", err)
		return
	}

	day--
	if day > 0 {
		goto DO
	}
}
