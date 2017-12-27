package main

import (
	"bytes"
	"errors"
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

	"github.com/moul/sshportal/pkg/bastionsession"
)

var (
	// Version should be updated by hand at each release
	Version = "1.6.0+dev"
	// GitTag will be overwritten automatically by the build system
	GitTag string
	// GitSha will be overwritten automatically by the build system
	GitSha string
	// GitBranch will be overwritten automatically by the build system
	GitBranch string
)

type sshportalContextKey string

var (
	userContextKey    = sshportalContextKey("user")
	messageContextKey = sshportalContextKey("message")
	errorContextKey   = sshportalContextKey("error")
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
					Name:  "config-user",
					Usage: "SSH user that spawns a configuration shell",
					Value: "admin",
				},
				cli.StringFlag{
					Name:  "healthcheck-user",
					Usage: "SSH user that returns healthcheck status without checking the SSH key",
					Value: "healthcheck",
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

	// ssh server
	shellHandler := func(s ssh.Session) {
		currentUser := s.Context().Value(userContextKey).(User)
		if s.User() != "healthcheck" {
			log.Printf("New connection(shell): sshUser=%q remote=%q local=%q command=%q dbUser=id:%q,email:%s", s.User(), s.RemoteAddr(), s.LocalAddr(), s.Command(), currentUser.ID, currentUser.Email)
		}

		if err2 := s.Context().Value(errorContextKey); err2 != nil {
			fmt.Fprintf(s, "error: %v\n", err2)
			return
		}

		if msg := s.Context().Value(messageContextKey); msg != nil {
			fmt.Fprint(s, msg.(string))
		}

		switch username := s.User(); {
		case username == c.String("healthcheck-user"):
			fmt.Fprintln(s, "OK")
			return
		case username == currentUser.Name || username == currentUser.Email || username == c.String("config-user"):
			if err = shell(c, s, s.Command(), db); err != nil {
				fmt.Fprintf(s, "error: %v\n", err)
			}
		case strings.HasPrefix(username, "invite:"):
			return
		default:
			fmt.Fprintf(s, "error: invalid user\n")
		}
	}

	bastionHandler := func(s ssh.Session) {
		currentUser := s.Context().Value(userContextKey).(User)
		log.Printf("New connection(bastion): sshUser=%q remote=%q local=%q command=%q dbUser=id:%q,email:%s", s.User(), s.RemoteAddr(), s.LocalAddr(), s.Command(), currentUser.ID, currentUser.Email)
		var host *Host
		host, err = RemoteHostFromSession(s, db)
		if err != nil {
			fmt.Fprintf(s, "error: %v\n", err)
			// FIXME: print available hosts
			return
		}

		// load up-to-date objects
		// FIXME: cache them or try not to load them
		var tmpUser User
		if err2 := db.Preload("Groups").Preload("Groups.ACLs").Where("id = ?", currentUser.ID).First(&tmpUser).Error; err2 != nil {
			fmt.Fprintf(s, "error: %v\n", err2)
			return
		}
		var tmpHost Host
		if err2 := db.Preload("Groups").Preload("Groups.ACLs").Where("id = ?", host.ID).First(&tmpHost).Error; err2 != nil {
			fmt.Fprintf(s, "error: %v\n", err2)
			return
		}

		action, err2 := CheckACLs(tmpUser, tmpHost)
		if err2 != nil {
			fmt.Fprintf(s, "error: %v\n", err2)
			return
		}

		// decrypt key and password
		HostDecrypt(c.String("aes-key"), host)
		SSHKeyDecrypt(c.String("aes-key"), host.SSHKey)

		switch action {
		case ACLActionAllow:
			sess := Session{
				UserID: currentUser.ID,
				HostID: host.ID,
				Status: SessionStatusActive,
			}
			if err2 := db.Create(&sess).Error; err2 != nil {
				fmt.Fprintf(s, "error: %v\n", err2)
				return
			}
			sessUpdate := Session{}
			if err2 := proxy(s, host, DynamicHostKey(db, host)); err2 != nil {
				fmt.Fprintf(s, "error: %v\n", err2)
				sessUpdate.ErrMsg = fmt.Sprintf("%v", err2)
				switch sessUpdate.ErrMsg {
				case "lch closed the connection", "rch closed the connection":
					sessUpdate.ErrMsg = ""
				}
			}
			sessUpdate.Status = SessionStatusClosed
			now := time.Now()
			sessUpdate.StoppedAt = &now
			db.Model(&sess).Updates(&sessUpdate)
		case ACLActionDeny:
			fmt.Fprintf(s, "You don't have permission to that host.\n")
		default:
			fmt.Fprintf(s, "error: invalid ACL action: %q\n", action)
		}
	}

	opts := []ssh.Option{}
	opts = append(opts, ssh.PasswordAuth(func(ctx ssh.Context, pass string) bool {
		ctx.SetValue(userContextKey, User{})
		return ctx.User() == "healthcheck"
	}))

	opts = append(opts, ssh.PublicKeyAuth(func(ctx ssh.Context, key ssh.PublicKey) bool {
		var (
			userKey  UserKey
			user     User
			username = ctx.User()
		)

		// lookup user by key
		db.Where("authorized_key = ?", string(gossh.MarshalAuthorizedKey(key))).First(&userKey)
		if userKey.UserID > 0 {
			db.Preload("Roles").Where("id = ?", userKey.UserID).First(&user)
			if strings.HasPrefix(username, "invite:") {
				ctx.SetValue(errorContextKey, fmt.Errorf("invites are only supported for new SSH keys; your ssh key is already associated with the user %q", user.Email))
			}
			ctx.SetValue(userContextKey, user)
			return true
		}

		// handle invite "links"
		if strings.HasPrefix(username, "invite:") {
			inputToken := strings.Split(username, ":")[1]
			if len(inputToken) > 0 {
				db.Where("invite_token = ?", inputToken).First(&user)
			}
			if user.ID > 0 {
				userKey = UserKey{
					UserID:        user.ID,
					Key:           key.Marshal(),
					Comment:       "created by sshportal",
					AuthorizedKey: string(gossh.MarshalAuthorizedKey(key)),
				}
				db.Create(&userKey)

				// token is only usable once
				user.InviteToken = ""
				db.Model(&user).Updates(&user)

				ctx.SetValue(messageContextKey, fmt.Sprintf("Welcome %s!\n\nYour key is now associated with the user %q.\n", user.Name, user.Email))
				ctx.SetValue(userContextKey, user)
			} else {
				ctx.SetValue(userContextKey, User{Name: "Anonymous"})
				ctx.SetValue(errorContextKey, errors.New("your token is invalid or expired"))
			}
			return true
		}

		// fallback
		ctx.SetValue(errorContextKey, errors.New("unknown ssh key"))
		ctx.SetValue(userContextKey, User{Name: "Anonymous"})
		return true
	}))

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

	log.Printf("info: SSH Server accepting connections on %s", c.String("bind-address"))
	ln, err := net.Listen("tcp", c.String("bind-address"))
	if err != nil {
		return err
	}
	srv := &ssh.Server{Addr: c.String("bind-address"), Handler: shellHandler}
	for _, opt := range opts {
		if err := srv.SetOption(opt); err != nil {
			return err
		}
	}
	srv.Version = fmt.Sprintf("sshportal-%s", Version)
	srv.ChannelHandler = func(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
		if newChan.ChannelType() != "session" {
			if err := newChan.Reject(gossh.UnknownChannelType, "unsupported channel type"); err != nil {
				log.Printf("error: failed to reject channel: %v", err)
			}
			return
		}
		// TODO: handle direct-tcp

		currentUser := ctx.Value(userContextKey).(User)
		username := conn.User()
		if username == c.String("healthcheck-user") || username == currentUser.Name || username == currentUser.Email || username == c.String("config-user") || strings.HasPrefix(username, "invite:") {
			ssh.DefaultChannelHandler(srv, conn, newChan, ctx)
		} else {
			bastionsession.ChannelHandler(srv, conn, newChan, ctx, bastionHandler)
		}
	}
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
