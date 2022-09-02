package connWrap

import (
	"errors"
	"fmt"
	"tide/pkg"
)

var ErrTimeout = errors.New("timeout")

type ErrorType int

const (
	ErrIO ErrorType = iota
	ErrParse
	ErrDevice
	ErrItem
)

// Error records an error and the operation and input that caused it.
type Error struct {
	Type     ErrorType
	Send     []byte
	Received []byte
	Err      error
}

func (e *Error) Error() string {
	var ret string
	ret = e.Err.Error()
	if e.Send != nil {
		ret += fmt.Sprintf(", Send: [% X]%s", e.Send, pkg.Printable(e.Send))
	}
	if e.Received != nil {
		ret += fmt.Sprintf(", Rcvd: [% X]%s", e.Received, pkg.Printable(e.Received))
	}
	return ret
}

func (e *Error) Unwrap() error { return e.Err }
