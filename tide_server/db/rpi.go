package db

import (
	"github.com/google/uuid"
	"time"
)

func SaveRpiStatus(stationId uuid.UUID, cpuTemp float64, t time.Time) error {
	_, err := TideDB.Exec("insert into rpi_status_log(station_id, cpu_temp, timestamp) VALUES ($1,$2,$3)", stationId, cpuTemp, t)
	return err
}
