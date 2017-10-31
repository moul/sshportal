package main

import (
	"fmt"
	"io"

	"golang.org/x/crypto/ssh/terminal"

	shlex "github.com/anmitsu/go-shlex"
	"github.com/gliderlabs/ssh"
	"github.com/urfave/cli"
)

var banner = `

    __________ _____           __       __
   / __/ __/ // / _ \___  ____/ /____ _/ /
  _\ \_\ \/ _  / ___/ _ \/ __/ __/ _ '/ /
 /___/___/_//_/_/   \___/_/  \__/\_,_/_/


`

func shell(s ssh.Session, sshCommand []string) error {
	if len(sshCommand) == 0 {
		io.WriteString(s, banner)
	}

	cli.AppHelpTemplate = `COMMANDS:
{{range .Commands}}{{if not .HideHelp}}   {{join .Names ", "}}{{ "\t"}}{{.Usage}}{{ "\n" }}{{end}}{{end}}{{if .VisibleFlags}}
GLOBAL OPTIONS:
   {{range .VisibleFlags}}{{.}}
   {{end}}{{end}}
`
	cli.OsExiter = func(c int) {
		// FIXME: forward valid exit code
		io.WriteString(s, fmt.Sprintf("exit: %d\n", c))
	}
	app := cli.NewApp()
	app.Writer = s
	app.HideVersion = true
	app.Commands = []cli.Command{
		{
			Name:  "host",
			Usage: "Manage hosts",
			Subcommands: []cli.Command{
				{
					Name:   "create",
					Usage:  "Create a new host",
					Action: func(c *cli.Context) error { return nil },
				},
				{
					Name:   "inspect",
					Usage:  "Display detailed information on one or more hosts",
					Action: func(c *cli.Context) error { return nil },
				},
				{
					Name:   "ls",
					Usage:  "List hosts",
					Action: func(c *cli.Context) error { return nil },
				},
				{
					Name:   "rm",
					Usage:  "Remove one or more hosts",
					Action: func(c *cli.Context) error { return nil },
				},
			},
		}, {
			Name:   "info",
			Usage:  "Display system-wide information",
			Action: func(c *cli.Context) error { return nil },
		}, {
			Name:  "key",
			Usage: "Manage keys",
			Subcommands: []cli.Command{
				{
					Name:   "create",
					Usage:  "Create a new key",
					Action: func(c *cli.Context) error { return nil },
				},
				{
					Name:   "inspect",
					Usage:  "Display detailed information on one or more keys",
					Action: func(c *cli.Context) error { return nil },
				},
				{
					Name:   "ls",
					Usage:  "List keys",
					Action: func(c *cli.Context) error { return nil },
				},
				{
					Name:   "rm",
					Usage:  "Remove one or more keys",
					Action: func(c *cli.Context) error { return nil },
				},
			},
		}, {
			Name:  "user",
			Usage: "Manage users",
			Subcommands: []cli.Command{
				{
					Name:   "create",
					Usage:  "Create a new user",
					Action: func(c *cli.Context) error { return nil },
				},
				{
					Name:   "inspect",
					Usage:  "Display detailed information on one or more users",
					Action: func(c *cli.Context) error { return nil },
				},
				{
					Name:   "ls",
					Usage:  "List users",
					Action: func(c *cli.Context) error { return nil },
				},
				{
					Name:   "rm",
					Usage:  "Remove one or more users",
					Action: func(c *cli.Context) error { return nil },
				},
			},
		},
	}

	if len(sshCommand) == 0 { // interactive mode
		term := terminal.NewTerminal(s, "config> ")
		for {
			line, err := term.ReadLine()
			if err != nil {
				return err
			}

			words, err := shlex.Split(line, true)
			if err != nil {
				io.WriteString(s, "syntax error.\n")
				continue
			}
			app.Run(append([]string{"config"}, words...))
		}
	} else { // oneshot mode
		app.Run(append([]string{"config"}, sshCommand...))
	}

	return nil
}
