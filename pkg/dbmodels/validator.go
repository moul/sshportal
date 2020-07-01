package dbmodels

import (
	"regexp"

	"github.com/asaskevich/govalidator"
)

func InitValidator() {
	unixUserRegexp := regexp.MustCompile("[a-z_][a-z0-9_-]*")

	govalidator.CustomTypeTagMap.Set("unix_user", govalidator.CustomTypeValidator(func(i interface{}, context interface{}) bool {
		name, ok := i.(string)
		if !ok {
			return false
		}
		return unixUserRegexp.MatchString(name)
	}))
}
