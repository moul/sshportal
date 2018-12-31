package main

import (
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/moul/ssh"
	"github.com/urfave/cli"
	gossh "golang.org/x/crypto/ssh"
	"moul.io/sshportal/pkg/bastion"
)

type serverConfig struct {
	aesKey          string
	dbDriver, dbURL string
	logsLocation    string
	bindAddr        string
	debug, demo     bool
	idleTimeout     time.Duration
}

func parseServerConfig(c *cli.Context) (*serverConfig, error) {
	ret := &serverConfig{
		aesKey:       c.String("aes-key"),
		dbDriver:     c.String("db-driver"),
		dbURL:        c.String("db-conn"),
		bindAddr:     c.String("bind-address"),
		debug:        c.Bool("debug"),
		demo:         c.Bool("demo"),
		logsLocation: c.String("logs-location"),
		idleTimeout:  c.Duration("idle-timeout"),
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

func server(c *serverConfig) (err error) {
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
	if err = bastion.DBInit(db); err != nil {
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
		Handler: func(s ssh.Session) { bastion.ShellHandler(s, Version, GitSha, GitTag, GitBranch) }, // ssh.Server.Handler is the handler for the DefaultSessionHandler
		Version: fmt.Sprintf("sshportal-%s", Version),
	}

	// configure channel handler
	defaultSessionHandler := srv.GetChannelHandler("session")
	defaultDirectTcpipHandler := srv.GetChannelHandler("direct-tcpip")
	bastion.DefaultChannelHandler = func(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
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
	srv.SetChannelHandler("default", bastion.ChannelHandler)

	if c.idleTimeout != 0 {
		srv.IdleTimeout = c.idleTimeout
		// gliderlabs/ssh requires MaxTimeout to be non-zero if we want to use IdleTimeout.
		// So, set it to the max value, because we don't want a max timeout.
		srv.MaxTimeout = math.MaxInt64
	}

	for _, opt := range []ssh.Option{
		// custom PublicKeyAuth handler
		ssh.PublicKeyAuth(bastion.PublicKeyAuthHandler(db, c.logsLocation, c.aesKey, c.dbDriver, c.dbURL, c.bindAddr, c.demo)),
		ssh.PasswordAuth(bastion.PasswordAuthHandler(db, c.logsLocation, c.aesKey, c.dbDriver, c.dbURL, c.bindAddr, c.demo)),
		// retrieve sshportal SSH private key from database
		bastion.PrivateKeyFromDB(db, c.aesKey),
	} {
		if err := srv.SetOption(opt); err != nil {
			return err
		}
	}

	log.Printf("info: SSH Server accepting connections on %s, idle-timout=%v", c.bindAddr, c.idleTimeout)
	return srv.Serve(ln)
}
