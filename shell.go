package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"regexp"
	"strings"

	shlex "github.com/anmitsu/go-shlex"
	"github.com/gliderlabs/ssh"
	"github.com/jinzhu/gorm"
	"github.com/moby/moby/pkg/namesgenerator"
	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/terminal"
)

var banner = `

    __________ _____           __       __
   / __/ __/ // / _ \___  ____/ /____ _/ /
  _\ \_\ \/ _  / ___/ _ \/ __/ __/ _ '/ /
 /___/___/_//_/_/   \___/_/  \__/\_,_/_/


`
var isNameValid = regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString

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
						cli.StringFlag{Name: "name", Usage: "Assign a name to the host"},
						cli.StringFlag{Name: "password", Usage: "If present, sshportal will use password-based authentication"},
						cli.StringFlag{Name: "fingerprint", Usage: "SSH host key fingerprint"},
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
					Name:        "create",
					Usage:       "Create a new key",
					Description: "$> key create\n   $> key create --name=mykey",
					Flags: []cli.Flag{
						cli.StringFlag{Name: "name", Usage: "Assign a name to the host"},
						cli.StringFlag{Name: "type", Value: "rsa"},
						cli.UintFlag{Name: "length", Value: 2048},
					},
					Action: func(c *cli.Context) error {
						key := SSHKey{}
						key.Name = namesgenerator.GetRandomName(0)
						if c.String("name") != "" {
							key.Name = c.String("name")
						}
						if key.Name == "" || !isNameValid(key.Name) {
							return fmt.Errorf("invalid name %q", key.Name)
						}
						key.Length = c.Uint("length")
						key.Type = c.String("type")

						// generate the ssh key
						if key.Type != "rsa" {
							return fmt.Errorf("key type not supported: %q", key.Type)
						}
						privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
						if err != nil {
							return err
						}
						// convert priv key to x509 format
						var pemKey = &pem.Block{
							Type:  "PRIVATE KEY",
							Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
						}
						buf := bytes.NewBufferString("")
						if err = pem.Encode(buf, pemKey); err != nil {
							return err
						}
						key.PrivKey = buf.String()
						//
						pub, err := gossh.NewPublicKey(&privateKey.PublicKey)
						if err != nil {
							return err
						}
						key.PubKey = strings.TrimSpace(string(gossh.MarshalAuthorizedKey(pub)))

						// save the key in database
						if err := db.Create(&key).Error; err != nil {
							return err
						}
						fmt.Fprintf(s, "%d\n", key.ID)
						return nil
					},
				},
				{
					Name:      "inspect",
					Usage:     "Display detailed information on one or more keys",
					ArgsUsage: "<id or name> [<id or name> [<ir or name>...]]",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return fmt.Errorf("invalid usage")
						}

						keys, err := FindKeysByIdOrName(db, c.Args())
						if err != nil {
							return nil
						}

						enc := json.NewEncoder(s)
						enc.SetIndent("", "  ")
						return enc.Encode(keys)
					},
				},
				{
					Name:  "ls",
					Usage: "List keys",
					Action: func(c *cli.Context) error {
						var keys []SSHKey
						if err := db.Find(&keys).Error; err != nil {
							return err
						}
						table := tablewriter.NewWriter(s)
						table.SetHeader([]string{"ID", "Name", "Type", "Length", "Fingerprint"})
						table.SetBorder(false)
						table.SetCaption(true, fmt.Sprintf("Total: %d keys.", len(keys)))
						for _, key := range keys {
							table.Append([]string{
								fmt.Sprintf("%d", key.ID),
								key.Name,
								key.Type,
								fmt.Sprintf("%d", key.Length),
								key.Fingerprint,
								//FIXME: add some stats
							})
						}
						table.Render()
						return nil
					},
				},
				{
					Name:      "rm",
					Usage:     "Remove one or more keys",
					ArgsUsage: "<id or name> [<id or name> [<ir or name>...]]",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return fmt.Errorf("invalid usage")
						}

						keys, err := FindKeysByIdOrName(db, c.Args())
						if err != nil {
							return nil
						}

						for _, key := range keys {
							db.Where("id = ?", key.ID).Delete(&SSHKey{})
							fmt.Fprintf(s, "%d\n", key.ID)
						}
						return nil
					},
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
