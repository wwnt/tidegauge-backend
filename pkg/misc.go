package pkg

import (
	"bytes"
	"unicode"
)

// Must panics if err is not nil.
func Must(err error) {
	if err != nil {
		panic(err)
	}
}

func Must2(_ interface{}, err error) {
	if err != nil {
		panic(err)
	}
}

func Printable(str []byte) []byte {
	return bytes.Map(func(r rune) rune {
		if unicode.IsPrint(r) { // unicode.IsSpace(r)
			return r
		}
		return -1
	}, str)
}
