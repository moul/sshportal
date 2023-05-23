package main

import (
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"moul.io/sshportal/pkg/bastion"

	"github.com/gliderlabs/ssh"
	"github.com/urfave/cli"
	gossh "golang.org/x/crypto/ssh"
)

type serverConfig struct {
	aesKey          string
	dbDriver, dbURL string
	logsLocation    string
	bindAddr        string
	debug, demo     bool
	idleTimeout     time.Duration
	aclCheckCmd     string
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
		aclCheckCmd:  c.String("acl-check-cmd"),
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

func dbConnect(c *serverConfig, config gorm.Option) (*gorm.DB, error) {
	var dbOpen func(string) gorm.Dialector
	if c.dbDriver == "sqlite3" {
		dbOpen = sqlite.Open
	}
	if c.dbDriver == "postgres" {
		dbOpen = postgres.Open
	}

	if c.dbDriver == "mysql" {
		dbOpen = mysql.Open
	}
	return gorm.Open(dbOpen(c.dbURL), config)
}

func server(c *serverConfig) (err error) {
	// configure db logging

	db, _ := dbConnect(c, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	sqlDB, err := db.DB()

	defer func() {
		origErr := err
		err = sqlDB.Close()
		if origErr != nil {
			err = origErr
		}
	}()

	if err = sqlDB.Ping(); err != nil {
		return
	}

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
		Handler: func(s ssh.Session) { bastion.ShellHandler(s, GitTag, GitSha, GitTag) }, // ssh.Server.Handler is the handler for the DefaultSessionHandler
		Version: fmt.Sprintf("sshportal-%s", GitTag),
		ChannelHandlers: map[string]ssh.ChannelHandler{
			"default": bastion.ChannelHandler,
		},
	}

	// configure channel handler
	bastion.DefaultChannelHandler = func(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
		switch newChan.ChannelType() {
		case "session":
			go ssh.DefaultSessionHandler(srv, conn, newChan, ctx)
		case "direct-tcpip":
			go ssh.DirectTCPIPHandler(srv, conn, newChan, ctx)
		default:
			if err := newChan.Reject(gossh.UnknownChannelType, "unsupported channel type"); err != nil {
				log.Printf("failed to reject chan: %v", err)
			}
		}
	}

	if c.idleTimeout != 0 {
		srv.IdleTimeout = c.idleTimeout
		// gliderlabs/ssh requires MaxTimeout to be non-zero if we want to use IdleTimeout.
		// So, set it to the max value, because we don't want a max timeout.
		srv.MaxTimeout = math.MaxInt64
	}

	for _, opt := range []ssh.Option{
		// custom PublicKeyAuth handler
		ssh.PublicKeyAuth(bastion.PublicKeyAuthHandler(db, c.logsLocation, c.aclCheckCmd, c.aesKey, c.dbDriver, c.dbURL, c.bindAddr, c.demo)),
		ssh.PasswordAuth(bastion.PasswordAuthHandler(db, c.logsLocation, c.aclCheckCmd, c.aesKey, c.dbDriver, c.dbURL, c.bindAddr, c.demo)),
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
