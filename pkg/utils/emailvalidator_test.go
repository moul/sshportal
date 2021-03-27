package utils

import (
	"testing"
)

func TestEmailValidator(t *testing.T) {

	goodEmail := "goodemail@email.com"
	badEmail := "b@2323.22"

	got1 := ValidateEmail(goodEmail)
	if got1 == false {
		t.Errorf("got1= %v; want true", got1)
	}

	got2 := ValidateEmail(badEmail)
	if got2 == false {
		t.Errorf("got2= %v; want false", got2)
	}

}
