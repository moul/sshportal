package main // import "moul.io/sshportal"

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"net"
	"os"
	"path"
	"time"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/moul/ssh"
	"github.com/urfave/cli"
	gossh "golang.org/x/crypto/ssh"
)

var (
	// Version should be updated by hand at each release
	Version = "1.8.0+dev"
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
	app.Email = "https://moul.io/sshportal"
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
				cli.DurationFlag{
					Name:  "idle-timeout",
					Value: 0,
					Usage: "Duration before an inactive connection is timed out (0 to disable)",
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

var defaultChannelHandler ssh.ChannelHandler

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
		Addr:    c.bindAddr,
		Handler: shellHandler, // ssh.Server.Handler is the handler for the DefaultSessionHandler
		Version: fmt.Sprintf("sshportal-%s", Version),
	}

	// configure channel handler
	defaultSessionHandler := srv.GetChannelHandler("session")
	defaultDirectTcpipHandler := srv.GetChannelHandler("direct-tcpip")
	defaultChannelHandler = func(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
		switch newChan.ChannelType() {
		case "session":
			go defaultSessionHandler(srv, conn, newChan, ctx)
		case "direct-tcpip":
			go defaultDirectTcpipHandler(srv, conn, newChan, ctx)
		default:
			if err := newChan.Reject(gossh.UnknownChannelType, "unsupported channel type"); err != nil {
				log.Printf("failed to reject chan: %v", err)
			}
		}
	}
	srv.SetChannelHandler("session", nil)
	srv.SetChannelHandler("direct-tcpip", nil)
	srv.SetChannelHandler("default", channelHandler)

	if c.idleTimeout != 0 {
		srv.IdleTimeout = c.idleTimeout
		// gliderlabs/ssh requires MaxTimeout to be non-zero if we want to use IdleTimeout.
		// So, set it to the max value, because we don't want a max timeout.
		srv.MaxTimeout = math.MaxInt64
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

	log.Printf("info: SSH Server accepting connections on %s, idle-timout=%v", c.bindAddr, c.idleTimeout)
	return srv.Serve(ln)
}
