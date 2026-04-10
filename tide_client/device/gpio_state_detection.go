//go:build linux

package device

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"strings"
	"syscall"
	"tide/common"
	"tide/pkg"
	"tide/pkg/project"
	"time"

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

type gpioStateConfig struct {
	DeviceName         string `json:"device_name"`
	Pin                int    `json:"pin"`
	ItemName           string `json:"item_name"`
	ItemType           string `json:"item_type"`
	ActiveLow          bool   `json:"active_low"`
	Bias               string `json:"bias"`
	ReportInitialValue *bool  `json:"report_initial_value"`
}

func parseGPIOBias(bias string) (gpiocdev.LineReqOption, string) {
	switch strings.ToLower(bias) {
	case "", "pull_up":
		return gpiocdev.WithPullUp, "pull_up"
	case "pull_down":
		return gpiocdev.WithPullDown, "pull_down"
	case "disabled":
		return gpiocdev.WithBiasDisabled, "disabled"
	case "as_is":
		return gpiocdev.WithBiasAsIs, "as_is"
	default:
		slog.Error("Invalid gpio bias", "bias", bias, "allowed", []string{"pull_up", "pull_down", "disabled", "as_is"})
		os.Exit(1)
		return gpiocdev.WithPullUp, "pull_up"
	}
}

func init() {
	RegisterDevice("GPIOStateDetection", gpioStateDetection{})
}

type gpioStateDetection struct{}

func (gpioStateDetection) NewGPIODevice(gpio *gpiocdev.Chip, rawConf json.RawMessage) common.StringMapMap {
	var conf gpioStateConfig
	pkg.Must(json.Unmarshal(rawConf, &conf))
	if conf.ItemType == "" {
		conf.ItemType = "gpio_state"
	}
	reportInitialValue := true
	if conf.ReportInitialValue != nil {
		reportInitialValue = *conf.ReportInitialValue
	}
	biasOpt, biasName := parseGPIOBias(conf.Bias)

	opts := []gpiocdev.LineReqOption{biasOpt, gpiocdev.WithBothEdges,
		gpiocdev.WithDebounce(time.Millisecond * 10),
		gpiocdev.WithEventHandler(func(evt gpiocdev.LineEvent) {
			value := 0.0
			if evt.Type == gpiocdev.LineEventRisingEdge {
				value = 1
			}
			DataReceive <- []itemData{{
				At:       nowMs(),
				Typ:      common.MsgGpioData,
				ItemName: conf.ItemName,
				Value:    new(value),
			}}
		})}
	if conf.ActiveLow {
		opts = append(opts, gpiocdev.AsActiveLow)
	}

	ll, err := gpio.RequestLines([]int{conf.Pin}, opts...)
	if err != nil {
		if errors.Is(err, syscall.Errno(22)) && biasName != "as_is" {
			slog.Error("Note that gpio bias options require kernel V5.5 or later - check your kernel version.", "bias", biasName)
		}
		slog.Error("RequestLine returned error", "error", err)
		os.Exit(1)
	}
	slog.Info("watch on gpio pin", "pin", conf.Pin)
	project.RegisterReleaseFunc(func() { _ = ll.Close() })
	if reportInitialValue {
		values := []int{0}
		if err = ll.Values(values); err != nil {
			slog.Error("Failed to read initial gpio value", "pin", conf.Pin, "error", err)
			os.Exit(1)
		}
		DataReceive <- []itemData{{
			At:       nowMs(),
			Typ:      common.MsgGpioData,
			ItemName: conf.ItemName,
			Value:    new(float64(values[0])),
		}}
	}

	return common.StringMapMap{conf.DeviceName: {conf.ItemType: conf.ItemName}}
}
