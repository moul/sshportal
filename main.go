package main

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"path"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/urfave/cli"
)

var (
	// Version should be updated by hand at each release
	Version = "1.7.1+dev"
	// GitTag will be overwritten automatically by the build system
	GitTag string
	// GitSha will be overwritten automatically by the build system
	GitSha string
	// GitBranch will be overwritten automatically by the build system
	GitBranch string
)

func main() {
	rand.Seed(time.Now().UnixNano())

	app := cli.NewApp()
	app.Name = path.Base(os.Args[0])
	app.Author = "Manfred Touron"
	app.Version = Version + " (" + GitSha + ")"
	app.Email = "https://github.com/moul/sshportal"
	app.Commands = []cli.Command{
		{
			Name:  "server",
			Usage: "Start sshportal server",
			Action: func(c *cli.Context) error {
				if err := ensureLogDirectory(c.String("logs-location")); err != nil {
					return err
				}
				cfg, err := parseServeConfig(c)
				if err != nil {
					return err
				}
				return server(cfg)
			},
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:   "bind-address, b",
					EnvVar: "SSHPORTAL_BIND",
					Value:  ":2222",
					Usage:  "SSH server bind address",
				},
				cli.StringFlag{
					Name:  "db-driver",
					Value: "sqlite3",
					Usage: "GORM driver (sqlite3)",
				},
				cli.StringFlag{
					Name:  "db-conn",
					Value: "./sshportal.db",
					Usage: "GORM connection string",
				},
				cli.BoolFlag{
					Name:  "debug, D",
					Usage: "Display debug information",
				},
				cli.StringFlag{
					Name:  "aes-key",
					Usage: "Encrypt sensitive data in database (length: 16, 24 or 32)",
				},
				cli.StringFlag{
					Name:  "logs-location",
					Value: "./log",
					Usage: "Store user session files",
				},
			},
		}, {
			Name:   "healthcheck",
			Action: func(c *cli.Context) error { return healthcheck(c.String("addr"), c.Bool("wait"), c.Bool("quiet")) },
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "addr, a",
					Value: "localhost:2222",
					Usage: "sshportal server address",
				},
				cli.BoolFlag{
					Name:  "wait, w",
					Usage: "Loop indefinitely until sshportal is ready",
				},
				cli.BoolFlag{
					Name:  "quiet, q",
					Usage: "Do not print errors, if any",
				},
			},
		}, {
			Name:   "_test_server",
			Hidden: true,
			Action: testServer,
		},
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatalf("error: %v", err)
	}
}

func server(c *configServe) (err error) {
	var db = (*gorm.DB)(nil)

	// try to setup the local DB
	if db, err = gorm.Open(c.dbDriver, c.dbURL); err != nil {
		return
	}
	defer func() {
		origErr := err
		err = db.Close()
		if origErr != nil {
			err = origErr
		}
	}()
	if err = db.DB().Ping(); err != nil {
		return
	}
	db.LogMode(c.debug)
	if err = dbInit(db); err != nil {
		return
	}

	// create TCP listening socket
	ln, err := net.Listen("tcp", c.bindAddr)
	if err != nil {
		return err
	}

	// configure server
	srv := &ssh.Server{
		Addr:           c.bindAddr,
		Handler:        shellHandler, // ssh.Server.Handler is the handler for the DefaultSessionHandler
		Version:        fmt.Sprintf("sshportal-%s", Version),
		ChannelHandler: channelHandler,
	}

	for _, opt := range []ssh.Option{
		// custom PublicKeyAuth handler
		ssh.PublicKeyAuth(publicKeyAuthHandler(db, c)),
		ssh.PasswordAuth(passwordAuthHandler(db, c)),
		// retrieve sshportal SSH private key from database
		privateKeyFromDB(db, c.aesKey),
	} {
		if err := srv.SetOption(opt); err != nil {
			return err
		}
	}

	log.Printf("info: SSH Server accepting connections on %s", c.bindAddr)
	return srv.Serve(ln)
}
