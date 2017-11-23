package main

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
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
	// VERSION should be updated by hand at each release
	VERSION = "1.3.0+dev"
	// GIT_TAG will be overwritten automatically by the build system
	GIT_TAG string
	// GIT_SHA will be overwritten automatically by the build system
	GIT_SHA string
	// GIT_BRANCH will be overwritten automatically by the build system
	GIT_BRANCH string
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
	app.Version = VERSION + " (" + GIT_SHA + ")"
	app.Email = "https://github.com/moul/sshportal"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "bind-address, b",
			EnvVar: "SSHPORTAL_BIND",
			Value:  ":2222",
			Usage:  "SSH server bind address",
		},
		cli.BoolFlag{
			Name:  "demo",
			Usage: "*unsafe* - demo mode: accept all connections",
		},
		/*cli.StringFlag{
			Name:  "db-driver",
			Value: "sqlite3",
			Usage: "GORM driver (sqlite3)",
		},*/
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
	}
	app.Action = server
	if err := app.Run(os.Args); err != nil {
		log.Fatalf("error: %v", err)
	}
}

func server(c *cli.Context) error {
	// db
	db, err := gorm.Open("sqlite3", c.String("db-conn"))
	if err != nil {
		return err
	}
	defer db.Close()
	if err = db.DB().Ping(); err != nil {
		return err
	}
	if c.Bool("debug") {
		db.LogMode(true)
	}
	if err := dbInit(db); err != nil {
		return err
	}
	if c.Bool("demo") {
		if err := dbDemo(db); err != nil {
			return err
		}
	}

	// ssh server
	ssh.Handle(func(s ssh.Session) {
		currentUser := s.Context().Value(userContextKey).(User)
		log.Printf("New connection: sshUser=%q remote=%q local=%q command=%q dbUser=id:%q,email:%s", s.User(), s.RemoteAddr(), s.LocalAddr(), s.Command(), currentUser.ID, currentUser.Email)

		if err := s.Context().Value(errorContextKey); err != nil {
			fmt.Fprintf(s, "error: %v\n", err)
			return
		}

		if msg := s.Context().Value(messageContextKey); msg != nil {
			fmt.Fprint(s, msg.(string))
		}

		switch username := s.User(); {
		case username == currentUser.Name || username == currentUser.Email || username == c.String("config-user"):
			if err := shell(c, s, s.Command(), db); err != nil {
				fmt.Fprintf(s, "error: %v\n", err)
			}
		case strings.HasPrefix(username, "invite:"):
			return
		default:
			host, err := RemoteHostFromSession(s, db)
			if err != nil {
				fmt.Fprintf(s, "error: %v\n", err)
				// FIXME: print available hosts
				return
			}

			// load up-to-date objects
			// FIXME: cache them or try not to load them
			var tmpUser User
			if err := db.Preload("Groups").Preload("Groups.ACLs").Where("id = ?", currentUser.ID).First(&tmpUser).Error; err != nil {
				fmt.Fprintf(s, "error: %v\n", err)
				return
			}
			var tmpHost Host
			if err := db.Preload("Groups").Preload("Groups.ACLs").Where("id = ?", host.ID).First(&tmpHost).Error; err != nil {
				fmt.Fprintf(s, "error: %v\n", err)
				return
			}

			action, err := CheckACLs(tmpUser, tmpHost)
			if err != nil {
				fmt.Fprintf(s, "error: %v\n", err)
				return
			}

			switch action {
			case "allow":
				if err := proxy(s, host); err != nil {
					fmt.Fprintf(s, "error: %v\n", err)
				}
			case "deny":
				fmt.Fprintf(s, "You don't have permission to that host.\n")
			default:
				fmt.Fprintf(s, "error: %v\n", err)
			}

		}
	})

	opts := []ssh.Option{}
	opts = append(opts, ssh.PublicKeyAuth(func(ctx ssh.Context, key ssh.PublicKey) bool {
		var (
			userKey  UserKey
			user     User
			username = ctx.User()
		)

		// lookup user by key
		db.Where("key = ?", key.Marshal()).First(&userKey)
		if userKey.UserID > 0 {
			db.Preload("Roles").Where("id = ?", userKey.UserID).First(&user)
			if strings.HasPrefix(username, "invite:") {
				ctx.SetValue(errorContextKey, fmt.Errorf("invites are only supported for ney SSH keys; your ssh key is already associated with the user %q.", user.Email))
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
					UserID:  user.ID,
					Key:     key.Marshal(),
					Comment: "created by sshportal",
				}
				db.Create(&userKey)

				// token is only usable once
				user.InviteToken = ""
				db.Update(&user)

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
		if err := SSHKeysByIdentifiers(db, []string{"host"}).First(&key).Error; err != nil {
			return err
		}
		signer, err := gossh.ParsePrivateKey([]byte(key.PrivKey))
		if err != nil {
			return err
		}
		srv.AddHostKey(signer)
		return nil
	})

	log.Printf("SSH Server accepting connections on %s", c.String("bind-address"))
	return ssh.ListenAndServe(c.String("bind-address"), nil, opts...)
}
