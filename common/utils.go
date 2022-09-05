package common

import "regexp"

var illegalCharacterReg = regexp.MustCompile("[^0-9a-z_]")

func ContainsIllegalCharacter(name string) bool {
	return illegalCharacterReg.MatchString(name)
}
