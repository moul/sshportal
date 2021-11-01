package main // import "moul.io/sshportal"

import (
	"log"
	"math/rand"
	"os"
	"path"

	"github.com/urfave/cli"
	"moul.io/srand"
)

var (
	// GitTag will be overwritten automatically by the build system
	GitTag = "n/a"
	// GitSha will be overwritten automatically by the build system
	GitSha = "n/a"
)

func main() {
	rand.Seed(srand.MustSecure())

	app := cli.NewApp()
	app.Name = path.Base(os.Args[0])
	app.Author = "Manfred Touron"
	app.Version = GitTag + " (" + GitSha + ")"
	app.Email = "https://moul.io/sshportal"
	app.Commands = []cli.Command{
		{
			Name:  "server",
			Usage: "Start sshportal server",
			Action: func(c *cli.Context) error {
				if err := ensureLogDirectory(c.String("logs-location")); err != nil {
					return err
				}
				cfg, err := parseServerConfig(c)
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
					Name:   "db-driver",
					EnvVar: "SSHPORTAL_DB_DRIVER",
					Value:  "sqlite3",
					Usage:  "GORM driver (sqlite3)",
				},
				cli.StringFlag{
					Name:   "db-conn",
					EnvVar: "SSHPORTAL_DATABASE_URL",
					Value:  "./sshportal.db",
					Usage:  "GORM connection string",
				},
				cli.BoolFlag{
					Name:   "debug, D",
					EnvVar: "SSHPORTAL_DEBUG",
					Usage:  "Display debug information",
				},
				cli.StringFlag{
					Name:   "aes-key",
					EnvVar: "SSHPORTAL_AES_KEY",
					Usage:  "Encrypt sensitive data in database (length: 16, 24 or 32)",
				},
				cli.StringFlag{
					Name:   "logs-location",
					EnvVar: "SSHPORTAL_LOGS_LOCATION",
					Value:  "./log",
					Usage:  "Store user session files",
				},
				cli.DurationFlag{
					Name:  "idle-timeout",
					Value: 0,
					Usage: "Duration before an inactive connection is timed out (0 to disable)",
				},
				cli.StringFlag{
					Name:   "acl-check-cmd",
					EnvVar: "SSHPORTAL_ACL_CHECK_CMD",
					Usage:  "Execute external command to check ACL",
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
