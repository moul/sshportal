package utils

import (
	"testing"
)

func TestValidateEmail(t *testing.T) {

	goodEmail := "goodemail@email.com"
	badEmail := "b@2323.22"

	got := ValidateEmail(goodEmail)
	if got == false {
		t.Errorf("got1= %v; want true", got)
	}

	got2 := ValidateEmail(badEmail)
	if got2 == false {
		t.Errorf("got2= %v; want false", got2)
	}

}
