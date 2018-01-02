package main

import (
	"bytes"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"path"
	"strings"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/urfave/cli"
	gossh "golang.org/x/crypto/ssh"
)

var (
	// Version should be updated by hand at each release
	Version = "1.7.0+dev"
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
			Name:   "server",
			Usage:  "Start sshportal server",
			Action: server,
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
			},
		}, {
			Name:   "healthcheck",
			Action: healthcheck,
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
		},
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatalf("error: %v", err)
	}
}

func server(c *cli.Context) error {
	switch len(c.String("aes-key")) {
	case 0, 16, 24, 32:
	default:
		return fmt.Errorf("invalid aes key size, should be 16 or 24, 32")
	}
	// db
	db, err := gorm.Open(c.String("db-driver"), c.String("db-conn"))
	if err != nil {
		return err
	}
	defer func() {
		if err2 := db.Close(); err2 != nil {
			panic(err2)
		}
	}()
	if err = db.DB().Ping(); err != nil {
		return err
	}
	if c.Bool("debug") {
		db.LogMode(true)
	}
	if err = dbInit(db); err != nil {
		return err
	}

	opts := []ssh.Option{}
	// custom PublicKeyAuth handler
	opts = append(opts, ssh.PublicKeyAuth(publicKeyAuthHandler(db, c)))
	opts = append(opts, ssh.PasswordAuth(passwordAuthHandler(db, c)))

	// retrieve sshportal SSH private key from databse
	opts = append(opts, func(srv *ssh.Server) error {
		var key SSHKey
		if err = SSHKeysByIdentifiers(db, []string{"host"}).First(&key).Error; err != nil {
			return err
		}
		SSHKeyDecrypt(c.String("aes-key"), &key)

		var signer gossh.Signer
		signer, err = gossh.ParsePrivateKey([]byte(key.PrivKey))
		if err != nil {
			return err
		}
		srv.AddHostKey(signer)
		return nil
	})

	// create TCP listening socket
	ln, err := net.Listen("tcp", c.String("bind-address"))
	if err != nil {
		return err
	}

	// configure server
	srv := &ssh.Server{
		Addr:           c.String("bind-address"),
		Handler:        shellHandler, // ssh.Server.Handler is the handler for the DefaultSessionHandler
		Version:        fmt.Sprintf("sshportal-%s", Version),
		ChannelHandler: channelHandler,
	}
	for _, opt := range opts {
		if err := srv.SetOption(opt); err != nil {
			return err
		}
	}

	log.Printf("info: SSH Server accepting connections on %s", c.String("bind-address"))
	return srv.Serve(ln)
}

// perform a healthcheck test without requiring an ssh client or an ssh key (used for Docker's HEALTHCHECK)
func healthcheck(c *cli.Context) error {
	config := gossh.ClientConfig{
		User:            "healthcheck",
		HostKeyCallback: func(hostname string, remote net.Addr, key gossh.PublicKey) error { return nil },
		Auth:            []gossh.AuthMethod{gossh.Password("healthcheck")},
	}

	if c.Bool("wait") {
		for {
			if err := healthcheckOnce(c.String("addr"), config, c.Bool("quiet")); err != nil {
				if !c.Bool("quiet") {
					log.Printf("error: %v", err)
				}
				time.Sleep(time.Second)
				continue
			}
			return nil
		}
	}

	if err := healthcheckOnce(c.String("addr"), config, c.Bool("quiet")); err != nil {
		if c.Bool("quiet") {
			return cli.NewExitError("", 1)
		}
		return err
	}
	return nil
}

func healthcheckOnce(addr string, config gossh.ClientConfig, quiet bool) error {
	client, err := gossh.Dial("tcp", addr, &config)
	if err != nil {
		return err
	}

	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer func() {
		if err := session.Close(); err != nil {
			if !quiet {
				log.Printf("failed to close session: %v", err)
			}
		}
	}()

	var b bytes.Buffer
	session.Stdout = &b
	if err := session.Run(""); err != nil {
		return err
	}
	stdout := strings.TrimSpace(b.String())
	if stdout != "OK" {
		return fmt.Errorf("invalid stdout: %q expected 'OK'", stdout)
	}
	return nil
}
