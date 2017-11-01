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

func main() {
	app := cli.NewApp()
	app.Name = path.Base(os.Args[0])
	app.Author = "Manfred Touron"
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
	}
	app.Action = server
	app.Run(os.Args)
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
		case "config":
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
		return errors.New("POC: real authentication is not yet implemented")
	}

	log.Printf("SSH Server accepting connections on %s", c.String("bind-address"))
	return ssh.ListenAndServe(c.String("bind-address"), nil, opts...)
}
