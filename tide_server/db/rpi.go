package db

import "time"

func SaveRpiStatus(cpuTemp, gpuTemp float64, t time.Time) error {
	_, err := TideDB.Exec("insert into rpi_status_log(cpu_temp, gpu_temp, timestamp) VALUES ($1,$2,$3)", cpuTemp, gpuTemp, t)
	return err
}
