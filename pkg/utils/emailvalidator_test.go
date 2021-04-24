package utils_test

import (
	"testing"

	"moul.io/sshportal/pkg/utils"
)

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"goodemail@email.com", true},
		{"b@2323.22", true},
		{"b@2322.", false},
		{"", false},
		{"blah", false},
		{"blah.com", false},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			got := utils.ValidateEmail(test.input)
			if got != test.expected {
				t.Errorf("expected %v, got %v", test.expected, got)
			}
		})
	}
}
