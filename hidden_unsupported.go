// +build windows

package main

import (
	"fmt"

	"github.com/urfave/cli"
)

// testServer is an hidden handler used for integration tests
func testServer(c *cli.Context) error { return fmt.Errorf("not supported on this architecture") }
