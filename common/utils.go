package common

import "regexp"

var evilItemNameReg = regexp.MustCompile("[^0-9a-z_]")

func EvilItemName(name string) bool {
	return evilItemNameReg.MatchString(name)
}
