//go:build !linux

package tcp

import (
	"errors"
	"tide/tide_client/connWrap"
)

func StartTcp(_ string, _ uint32) (connWrap.ConnCommon, error) {
	return nil, errors.New("tcp connWrap is only supported on linux")
}
