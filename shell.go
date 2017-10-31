package main

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"

	"golang.org/x/crypto/ssh/terminal"

	shlex "github.com/anmitsu/go-shlex"
	"github.com/gliderlabs/ssh"
	"github.com/jinzhu/gorm"
	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli"
)

var banner = `

    __________ _____           __       __
   / __/ __/ // / _ \___  ____/ /____ _/ /
  _\ \_\ \/ _  / ___/ _ \/ __/ __/ _ '/ /
 /___/___/_//_/_/   \___/_/  \__/\_,_/_/


`
var isNameValid = regexp.MustCompile(`^[A-Za-z0-9-]+$`).MatchString

func shell(s ssh.Session, sshCommand []string, db *gorm.DB) error {
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
					Name:        "create",
					Usage:       "Create a new host",
					ArgsUsage:   "<user>[:<password>]@<host>[:<port>]",
					Description: "$> host create bob@example.com:2222",
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "name",
							Usage: "Assign a name to the host",
						},
						cli.StringFlag{
							Name:  "password",
							Usage: "If present, sshportal will use password-based authentication",
						},
						cli.StringFlag{
							Name:  "fingerprint",
							Usage: "SSH host key fingerprint",
						},
					},
					Action: func(c *cli.Context) error {
						if c.NArg() != 1 {
							return fmt.Errorf("invalid usage")
						}
						host, err := NewHostFromURL(c.Args().First())
						if err != nil {
							return err
						}
						if c.String("password") != "" {
							host.Password = c.String("password")
						}
						host.Fingerprint = c.String("fingerprint")
						host.Name = host.Hostname()
						if c.String("name") != "" {
							host.Name = c.String("name")
						}
						if !isNameValid(host.Name) {
							return fmt.Errorf("invalid name %q", host.Name)
						}
						if err := db.Create(&host).Error; err != nil {
							return err
						}
						fmt.Fprintf(s, "%d\n", host.ID)
						return nil
					},
				},
				{
					Name:      "inspect",
					Usage:     "Display detailed information on one or more hosts",
					ArgsUsage: "<id or name> [<id or name> [<ir or name>...]]",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return fmt.Errorf("invalid usage")
						}

						hosts, err := FindHostsByIdOrName(db, c.Args())
						if err != nil {
							return nil
						}

						enc := json.NewEncoder(s)
						enc.SetIndent("", "  ")
						return enc.Encode(hosts)
					},
				},
				{
					Name:  "ls",
					Usage: "List hosts",
					Action: func(c *cli.Context) error {
						var hosts []Host
						if err := db.Find(&hosts).Error; err != nil {
							return err
						}
						table := tablewriter.NewWriter(s)
						table.SetHeader([]string{"ID", "Name", "URL", "Password", "Fingerprint"})
						table.SetBorder(false)
						table.SetCaption(true, fmt.Sprintf("Total: %d hosts.", len(hosts)))
						for _, host := range hosts {
							table.Append([]string{
								fmt.Sprintf("%d", host.ID),
								host.Name,
								host.URL(),
								host.Password,
								host.Fingerprint,
								//host.PrivKey,
								//FIXME: add some stats about last access time etc
							})
						}
						table.Render()
						return nil
					},
				},
				{
					Name:      "rm",
					Usage:     "Remove one or more hosts",
					ArgsUsage: "<id or name> [<id or name> [<ir or name>...]]",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return fmt.Errorf("invalid usage")
						}

						hosts, err := FindHostsByIdOrName(db, c.Args())
						if err != nil {
							return nil
						}

						for _, host := range hosts {
							db.Where("id = ?", host.ID).Delete(&Host{})
							fmt.Fprintf(s, "%d\n", host.ID)
						}
						return nil
					},
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
			if err := app.Run(append([]string{"config"}, words...)); err != nil {
				io.WriteString(s, fmt.Sprintf("error: %v\n", err))
			}
		}
	} else { // oneshot mode
		if err := app.Run(append([]string{"config"}, sshCommand...)); err != nil {
			io.WriteString(s, fmt.Sprintf("error: %v\n", err))
		}
	}

	return nil
}
