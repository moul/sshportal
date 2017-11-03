package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"

	"github.com/gliderlabs/ssh"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/urfave/cli"
)

var version = "0.0.1"

type sshportalContextKey string

var userContextKey = sshportalContextKey("user")

func main() {
	app := cli.NewApp()
	app.Name = path.Base(os.Args[0])
	app.Author = "Manfred Touron"
	app.Version = version
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
		cli.StringFlag{
			Name:  "db-driver",
			Value: "sqlite3",
			Usage: "GORM driver (sqlite3, mysql)",
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
	}
	app.Action = server
	if err := app.Run(os.Args); err != nil {
		log.Fatalf("error: %v", err)
	}
}

func server(c *cli.Context) error {
	db, err := gorm.Open(c.String("db-driver"), c.String("db-conn"))
	if err != nil {
		return err
	}
	defer db.Close()
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

	ssh.Handle(func(s ssh.Session) {
		log.Printf("New connection: user=%q remote=%q local=%q command=%q", s.User(), s.RemoteAddr(), s.LocalAddr(), s.Command())

		switch s.User() {
		case c.String("config-user"):
			if err := shell(c, s, s.Command(), db); err != nil {
				io.WriteString(s, fmt.Sprintf("error: %v\n", err))
			}
		default:
			host, err := RemoteHostFromSession(s, db)
			if err != nil {
				io.WriteString(s, fmt.Sprintf("error: %v\n", err))
				// FIXME: print available hosts
				return
			}
			if err := proxy(s, host); err != nil {
				io.WriteString(s, fmt.Sprintf("error: %v\n", err))
			}
		}
	})

	opts := []ssh.Option{}
	if !c.Bool("demo") {
		return errors.New("use `--demo` for now")
	}

	opts = append(opts, ssh.PublicKeyAuth(func(ctx ssh.Context, key ssh.PublicKey) bool {
		var (
			userKey UserKey
			user    User
			count   uint
		)

		// lookup user by key
		db.Where("key = ?", key.Marshal()).First(&userKey)
		if userKey.UserID > 0 {
			db.Where("id = ?", userKey.UserID).First(&user)
			ctx.SetValue(userContextKey, user)
			return true
		}

		// check if there are users in DB
		db.Table("users").Count(&count)
		if count == 0 { // create an admin user
			// if no admin, create an account for the first connection
			user = User{
				Name:    "Administrator",
				Email:   "admin@sshportal",
				Comment: "created by sshportal",
				IsAdmin: true,
			}
			db.Create(&user)
			userKey = UserKey{
				UserID: user.ID,
				Key:    key.Marshal(),
			}
			db.Create(&userKey)

			ctx.SetValue(userContextKey, user)
			return true
		}

		return false
	}))

	log.Printf("SSH Server accepting connections on %s", c.String("bind-address"))
	return ssh.ListenAndServe(c.String("bind-address"), nil, opts...)
}
