// +build tools

package tools

import (
	// required by depaware
	_ "github.com/tailscale/depaware/depaware"

	// required by goimports
	_ "golang.org/x/tools/cover"
)
