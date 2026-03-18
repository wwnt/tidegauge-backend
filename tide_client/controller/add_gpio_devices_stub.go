//go:build !linux

package controller

import (
	"encoding/json"
	"log/slog"
	"os"
	"tide/common"
)

func init() {
	RegisterConn("gpio", newGpioConn)
}

func newGpioConn(_ json.RawMessage) common.StringMapMap {
	slog.Error("GPIO connections are only supported on linux")
	os.Exit(1)
	return nil
}
