//go:build linux

package device

import (
	"encoding/json"
	"log/slog"
	"os"
	"tide/common"

	"github.com/warthog618/go-gpiocdev"
)

type GPIODevice interface {
	NewGPIODevice(chip *gpiocdev.Chip, rawConf json.RawMessage) common.StringMapMap
}

func MustGPIODevice(name string) GPIODevice {
	device, ok := getRegisteredDevice(name).(GPIODevice)
	if !ok {
		slog.Error("Device model does not support requested capability", "model", name, "capability", "gpio")
		os.Exit(1)
		return nil
	}
	return device
}
