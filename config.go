package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli"
)

type configServe struct {
	aesKey          string
	dbDriver, dbURL string
	logsLocation    string
	bindAddr        string
	debug, demo     bool
}

func parseServeConfig(c *cli.Context) (*configServe, error) {
	ret := &configServe{
		aesKey:       c.String("aes-key"),
		dbDriver:     c.String("db-driver"),
		dbURL:        c.String("db-conn"),
		bindAddr:     c.String("bind-address"),
		debug:        c.Bool("debug"),
		demo:         c.Bool("demo"),
		logsLocation: c.String("logs-location"),
	}
	switch len(ret.aesKey) {
	case 0, 16, 24, 32:
	default:
		return nil, fmt.Errorf("invalid aes key size, should be 16 or 24, 32")
	}
	return ret, nil
}

func ensureLogDirectory(location string) error {
	// check for the logdir existence
	logsLocation, err := os.Stat(location)
	if err != nil {
		if os.IsNotExist(err) {
			return os.MkdirAll(location, os.ModeDir|os.FileMode(0750))
		}
		return err
	}
	if !logsLocation.IsDir() {
		return fmt.Errorf("log directory cannot be created")
	}
	return nil
}
