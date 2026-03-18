//go:build !linux

package device

import (
	"encoding/json"
	"log/slog"
	"os"
	"tide/common"
)

func init() {
	RegisterDevice("DoorIntrusionDetection", unsupportedGPIODevice("DoorIntrusionDetection"))
	RegisterDevice("DRD11A", unsupportedGPIODevice("DRD11A"))
	RegisterDevice("RG13", unsupportedGPIODevice("RG13"))
}

type unsupportedGPIODevice string

func (d unsupportedGPIODevice) NewDevice(_ any, _ json.RawMessage) common.StringMapMap {
	slog.Error("GPIO device model is only supported on linux", "model", string(d))
	os.Exit(1)
	return nil
}
