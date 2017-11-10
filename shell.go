package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"

	shlex "github.com/anmitsu/go-shlex"
	"github.com/gliderlabs/ssh"
	"github.com/jinzhu/gorm"
	"github.com/moby/moby/pkg/namesgenerator"
	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli"
	"golang.org/x/crypto/ssh/terminal"
)

var banner = `

    __________ _____           __       __
   / __/ __/ // / _ \___  ____/ /____ _/ /
  _\ \_\ \/ _  / ___/ _ \/ __/ __/ _ '/ /
 /___/___/_//_/_/   \___/_/  \__/\_,_/_/


`
var isNameValid = regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString
var startTime = time.Now()

func shell(globalContext *cli.Context, s ssh.Session, sshCommand []string, db *gorm.DB) error {
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
	cli.HelpFlag = cli.BoolFlag{
		Name:   "help, h",
		Hidden: true,
	}
	app := cli.NewApp()
	app.Writer = s
	app.HideVersion = true
	app.Commands = []cli.Command{
		{
			Name:  "host",
			Usage: "Manages hosts",
			Subcommands: []cli.Command{
				{
					Name:        "create",
					Usage:       "Creates a new host",
					ArgsUsage:   "<user>[:<password>]@<host>[:<port>]",
					Description: "$> host create bart@foo.org\n   $> host create bob:marley@example.com:2222",
					Flags: []cli.Flag{
						cli.StringFlag{Name: "name", Usage: "Assigns a name to the host"},
						cli.StringFlag{Name: "password", Usage: "If present, sshportal will use password-based authentication"},
						cli.StringFlag{Name: "fingerprint", Usage: "SSH host key fingerprint"},
						cli.StringFlag{Name: "comment"},
						cli.StringFlag{Name: "key", Usage: "ID or name of the key to use for authentication"},
						cli.StringFlag{Name: "group", Usage: "Name or ID of the host group", Value: "default"},
					},
					Action: func(c *cli.Context) error {
						if c.NArg() != 1 {
							return cli.ShowSubcommandHelp(c)
						}
						host, err := NewHostFromURL(c.Args().First())
						if err != nil {
							return err
						}
						if c.String("password") != "" {
							host.Password = c.String("password")
						}
						host.Fingerprint = c.String("fingerprint")
						host.Name = strings.Split(host.Hostname(), ".")[0]

						if c.String("name") != "" {
							host.Name = c.String("name")
						}
						if !isNameValid(host.Name) {
							return fmt.Errorf("invalid name %q", host.Name)
						}
						// FIXME: check if name already exists

						host.Comment = c.String("comment")
						inputKey := c.String("key")
						if inputKey == "" && host.Password == "" {
							inputKey = "default"
						}
						if inputKey != "" {
							key, err := FindKeyByIdOrName(db, inputKey)
							if err != nil {
								return err
							}
							host.SSHKeyID = key.ID
						}

						// host group
						hostGroup, err := FindHostGroupByIdOrName(db, c.String("group"))
						if err != nil {
							return err
						}
						host.Groups = []HostGroup{*hostGroup}

						if err := db.Create(&host).Error; err != nil {
							return err
						}
						fmt.Fprintf(s, "%d\n", host.ID)
						return nil
					},
				}, {
					Name:      "inspect",
					Usage:     "Shows detailed information on one or more hosts",
					ArgsUsage: "<id or name> [<id or name> [<ir or name>...]]",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						hosts, err := FindHostsByIdOrName(db, c.Args())
						if err != nil {
							return nil
						}

						enc := json.NewEncoder(s)
						enc.SetIndent("", "  ")
						return enc.Encode(hosts)
					},
				}, {
					Name:  "ls",
					Usage: "Lists hosts",
					Action: func(c *cli.Context) error {
						var hosts []Host
						if err := db.Preload("Groups").Find(&hosts).Error; err != nil {
							return err
						}
						table := tablewriter.NewWriter(s)
						table.SetHeader([]string{"ID", "Name", "URL", "Key", "Pass", "Groups", "Comment"})
						table.SetBorder(false)
						table.SetCaption(true, fmt.Sprintf("Total: %d hosts.", len(hosts)))
						for _, host := range hosts {
							authKey, authPass := "", ""
							if host.Password != "" {
								authPass = "X"
							}
							if host.SSHKeyID > 0 {
								var key SSHKey
								db.Model(&host).Related(&key)
								authKey = key.Name
							}
							table.Append([]string{
								fmt.Sprintf("%d", host.ID),
								host.Name,
								host.URL(),
								authKey,
								authPass,
								fmt.Sprintf("%d", len(host.Groups)),
								host.Comment,
								//FIXME: add some stats about last access time etc
								//FIXME: add creation date
							})
						}
						table.Render()
						return nil
					},
				}, {
					Name:      "rm",
					Usage:     "Removes one or more hosts",
					ArgsUsage: "<id or name> [<id or name> [<ir or name>...]]",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
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
			Name:  "hostgroup",
			Usage: "Manages host groups",
			Subcommands: []cli.Command{
				{
					Name:        "create",
					Usage:       "Creates a new host group",
					Description: "$> hostgroup create --name=prod",
					Flags: []cli.Flag{
						cli.StringFlag{Name: "name", Usage: "Assigns a name to the host group"},
						cli.StringFlag{Name: "comment"},
					},
					Action: func(c *cli.Context) error {
						hostGroup := HostGroup{
							Name: c.String("name"),
						}
						if hostGroup.Name == "" {
							hostGroup.Name = namesgenerator.GetRandomName(0)
						}
						if !isNameValid(hostGroup.Name) {
							return fmt.Errorf("invalid name %q", hostGroup.Name)
						}
						// FIXME: check if name already exists
						hostGroup.Comment = c.String("comment")

						if err := db.Create(&hostGroup).Error; err != nil {
							return err
						}
						fmt.Fprintf(s, "%d\n", hostGroup.ID)
						return nil
					},
				}, {
					Name:      "inspect",
					Usage:     "Shows detailed information on one or more host groups",
					ArgsUsage: "<id or name> [<id or name> [<ir or name>...]]",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						hostGroups, err := FindHostGroupsByIdOrName(db, c.Args())
						if err != nil {
							return nil
						}

						enc := json.NewEncoder(s)
						enc.SetIndent("", "  ")
						return enc.Encode(hostGroups)
					},
				}, {
					Name:  "ls",
					Usage: "Lists host groups",
					Action: func(c *cli.Context) error {
						var hostGroups []HostGroup
						if err := db.Preload("Hosts").Find(&hostGroups).Error; err != nil {
							return err
						}
						table := tablewriter.NewWriter(s)
						table.SetHeader([]string{"ID", "Name", "Hosts", "Comment"})
						table.SetBorder(false)
						table.SetCaption(true, fmt.Sprintf("Total: %d host groups.", len(hostGroups)))
						for _, hostGroup := range hostGroups {
							// FIXME: add more stats (amount of hosts, linked usergroups, ...)
							table.Append([]string{
								fmt.Sprintf("%d", hostGroup.ID),
								hostGroup.Name,
								fmt.Sprintf("%d", len(hostGroup.Hosts)),
								hostGroup.Comment,
							})
						}
						table.Render()
						return nil
					},
				}, {
					Name:      "rm",
					Usage:     "Removes one or more host groups",
					ArgsUsage: "<id or name> [<id or name> [<ir or name>...]]",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						hostGroups, err := FindHostGroupsByIdOrName(db, c.Args())
						if err != nil {
							return nil
						}

						for _, hostGroup := range hostGroups {
							db.Where("id = ?", hostGroup.ID).Delete(&HostGroup{})
							fmt.Fprintf(s, "%d\n", hostGroup.ID)
						}
						return nil
					},
				},
			},
		}, {
			Name:  "info",
			Usage: "Shows system-wide information",
			Action: func(c *cli.Context) error {
				fmt.Fprintf(s, "Debug mode (server): %v\n", globalContext.Bool("debug"))
				hostname, _ := os.Hostname()
				fmt.Fprintf(s, "Hostname: %s\n", hostname)
				fmt.Fprintf(s, "CPUs: %d\n", runtime.NumCPU())
				fmt.Fprintf(s, "Demo mode: %v\n", globalContext.Bool("demo"))
				fmt.Fprintf(s, "DB Driver: %s\n", globalContext.String("db-driver"))
				fmt.Fprintf(s, "DB Conn: %s\n", globalContext.String("db-conn"))
				fmt.Fprintf(s, "Bind Address: %s\n", globalContext.String("bind-address"))
				fmt.Fprintf(s, "System Time: %v\n", time.Now().Format(time.RFC3339Nano))
				fmt.Fprintf(s, "OS Type: %s\n", runtime.GOOS)
				fmt.Fprintf(s, "OS Architecture: %s\n", runtime.GOARCH)
				fmt.Fprintf(s, "Go routines: %d\n", runtime.NumGoroutine())
				fmt.Fprintf(s, "Go version (build): %v\n", runtime.Version())
				fmt.Fprintf(s, "Uptime: %v\n", time.Since(startTime))

				myself := s.Context().Value(userContextKey).(User)
				fmt.Fprintf(s, "User email: %v\n", myself.ID)
				fmt.Fprintf(s, "User email: %s\n", myself.Email)
				// FIXME: add version
				// FIXME: add info about current server (network, cpu, ram, OS)
				// FIXME: add info about current user
				// FIXME: add active connections
				// FIXME: stats
				return nil
			},
		}, {
			Name:  "key",
			Usage: "Manages keys",
			Subcommands: []cli.Command{
				{
					Name:        "create",
					Usage:       "Creates a new key",
					Description: "$> key create\n   $> key create --name=mykey",
					Flags: []cli.Flag{
						cli.StringFlag{Name: "name", Usage: "Assigns a name to the key"},
						cli.StringFlag{Name: "type", Value: "rsa"},
						cli.UintFlag{Name: "length", Value: 2048},
						cli.StringFlag{Name: "comment"},
					},
					Action: func(c *cli.Context) error {
						name := namesgenerator.GetRandomName(0)
						if c.String("name") != "" {
							name = c.String("name")
						}
						if name == "" || !isNameValid(name) {
							return fmt.Errorf("invalid name %q", name)
						}
						// FIXME: check if name already exists

						key, err := NewSSHKey(c.String("type"), c.Uint("length"))
						if err != nil {
							return err
						}
						key.Name = name
						key.Comment = c.String("comment")

						// save the key in database
						if err := db.Create(&key).Error; err != nil {
							return err
						}
						fmt.Fprintf(s, "%d\n", key.ID)
						return nil
					},
				}, {
					Name:      "inspect",
					Usage:     "Shows detailed information on one or more keys",
					ArgsUsage: "<id or name> [<id or name> [<ir or name>...]]",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						keys, err := FindKeysByIdOrName(db, c.Args())
						if err != nil {
							return nil
						}

						enc := json.NewEncoder(s)
						enc.SetIndent("", "  ")
						return enc.Encode(keys)
					},
				}, {
					Name:  "ls",
					Usage: "Lists keys",
					Action: func(c *cli.Context) error {
						var keys []SSHKey
						if err := db.Preload("Hosts").Find(&keys).Error; err != nil {
							return err
						}
						table := tablewriter.NewWriter(s)
						table.SetHeader([]string{"ID", "Name", "Type", "Length", "Hosts", "Comment"})
						table.SetBorder(false)
						table.SetCaption(true, fmt.Sprintf("Total: %d keys.", len(keys)))
						for _, key := range keys {
							table.Append([]string{
								fmt.Sprintf("%d", key.ID),
								key.Name,
								key.Type,
								fmt.Sprintf("%d", key.Length),
								//key.Fingerprint,
								fmt.Sprintf("%d", len(key.Hosts)),
								key.Comment,
								//FIXME: add some stats
								//FIXME: add creation date
							})
						}
						table.Render()
						return nil
					},
				}, {
					Name:      "rm",
					Usage:     "Removes one or more keys",
					ArgsUsage: "<id or name> [<id or name> [<ir or name>...]]",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
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
			Usage: "Manages users",
			Subcommands: []cli.Command{
				{
					Name:      "inspect",
					Usage:     "Shows detailed information on one or more users",
					ArgsUsage: "<id or email> [<id or email> [<ir or email>...]]",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						users, err := FindUsersByIdOrEmail(db, c.Args())
						if err != nil {
							return nil
						}

						enc := json.NewEncoder(s)
						enc.SetIndent("", "  ")
						return enc.Encode(users)
					},
				}, {
					Name:        "invite",
					ArgsUsage:   "<email>",
					Usage:       "Invites a new user",
					Description: "$> user invite bob@example.com\n   $> user invite --name=Robert bob@example.com",
					Flags: []cli.Flag{
						cli.StringFlag{Name: "name", Usage: "Assigns a name to the user"},
						cli.StringFlag{Name: "comment"},
						cli.StringFlag{Name: "group", Usage: "Name or ID of the user group", Value: "default"},
					},
					Action: func(c *cli.Context) error {
						if c.NArg() != 1 {
							return cli.ShowSubcommandHelp(c)
						}

						// FIXME: validate email

						email := c.Args().First()
						name := strings.Split(email, "@")[0]
						if c.String("name") != "" {
							name = c.String("name")
						}

						user := User{
							Name:        name,
							Email:       email,
							Comment:     c.String("comment"),
							InviteToken: RandStringBytes(16),
						}

						// user group
						userGroup, err := FindUserGroupByIdOrName(db, c.String("group"))
						if err != nil {
							return err
						}
						user.Groups = []UserGroup{*userGroup}

						// save the user in database
						if err := db.Create(&user).Error; err != nil {
							return err
						}
						fmt.Fprintf(s, "User %d created.\nTo associate this account with a key, use the following SSH user: 'invite-%s'.\n", user.ID, user.InviteToken)
						return nil
					},
				}, {
					Name:  "ls",
					Usage: "Lists users",
					Action: func(c *cli.Context) error {
						var users []User
						if err := db.Preload("Groups").Preload("Keys").Find(&users).Error; err != nil {
							return err
						}
						table := tablewriter.NewWriter(s)
						table.SetHeader([]string{"ID", "Name", "Email", "Keys", "Groups", "Comment"})
						table.SetBorder(false)
						table.SetCaption(true, fmt.Sprintf("Total: %d users.", len(users)))
						for _, user := range users {
							table.Append([]string{
								fmt.Sprintf("%d", user.ID),
								user.Name,
								user.Email,
								fmt.Sprintf("%d", len(user.Keys)),
								fmt.Sprintf("%d", len(user.Groups)),
								user.Comment,
								//FIXME: add some stats about last access time etc
								//FIXME: add creation date
							})
						}
						table.Render()
						return nil
					},
				}, {
					Name:      "rm",
					Usage:     "Removes one or more users",
					ArgsUsage: "<id or email> [<id or email> [<ir or email>...]]",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						users, err := FindUsersByIdOrEmail(db, c.Args())
						if err != nil {
							return nil
						}

						for _, user := range users {
							db.Where("id = ?", user.ID).Delete(&User{})
							fmt.Fprintf(s, "%d\n", user.ID)
						}
						return nil
					},
				},
			},
		}, {
			Name:  "usergroup",
			Usage: "Manages user groups",
			Subcommands: []cli.Command{
				{
					Name:        "create",
					Usage:       "Creates a new user group",
					Description: "$> usergroup create --name=prod",
					Flags: []cli.Flag{
						cli.StringFlag{Name: "name", Usage: "Assigns a name to the user group"},
						cli.StringFlag{Name: "comment"},
					},
					Action: func(c *cli.Context) error {
						userGroup := UserGroup{
							Name: c.String("name"),
						}
						if userGroup.Name == "" {
							userGroup.Name = namesgenerator.GetRandomName(0)
						}
						if !isNameValid(userGroup.Name) {
							return fmt.Errorf("invalid name %q", userGroup.Name)
						}
						// FIXME: check if name already exists
						userGroup.Comment = c.String("comment")

						// add myself to the new group
						myself := s.Context().Value(userContextKey).(User)
						// FIXME: use foreign key with ID to avoid updating the user with the context
						userGroup.Users = []User{myself}

						if err := db.Create(&userGroup).Error; err != nil {
							return err
						}
						fmt.Fprintf(s, "%d\n", userGroup.ID)
						return nil
					},
				}, {
					Name:      "inspect",
					Usage:     "Shows detailed information on one or more user groups",
					ArgsUsage: "<id or name> [<id or name> [<ir or name>...]]",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						userGroups, err := FindUserGroupsByIdOrName(db, c.Args())
						if err != nil {
							return nil
						}

						enc := json.NewEncoder(s)
						enc.SetIndent("", "  ")
						return enc.Encode(userGroups)
					},
				}, {
					Name:  "ls",
					Usage: "Lists user groups",
					Action: func(c *cli.Context) error {
						var userGroups []UserGroup
						if err := db.Preload("Users").Find(&userGroups).Error; err != nil {
							return err
						}
						table := tablewriter.NewWriter(s)
						table.SetHeader([]string{"ID", "Name", "Users", "Comment"})
						table.SetBorder(false)
						table.SetCaption(true, fmt.Sprintf("Total: %d user groups.", len(userGroups)))
						for _, userGroup := range userGroups {
							// FIXME: add more stats (amount of users, linked usergroups, ...)
							table.Append([]string{
								fmt.Sprintf("%d", userGroup.ID),
								userGroup.Name,
								fmt.Sprintf("%d", len(userGroup.Users)),
								userGroup.Comment,
							})
						}
						table.Render()
						return nil
					},
				}, {
					Name:      "rm",
					Usage:     "Removes one or more user groups",
					ArgsUsage: "<id or name> [<id or name> [<ir or name>...]]",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						userGroups, err := FindUserGroupsByIdOrName(db, c.Args())
						if err != nil {
							return nil
						}

						for _, userGroup := range userGroups {
							db.Where("id = ?", userGroup.ID).Delete(&UserGroup{})
							fmt.Fprintf(s, "%d\n", userGroup.ID)
						}
						return nil
					},
				},
			},
		}, {
			Name:  "userkey",
			Usage: "Manages userkeys",
			Subcommands: []cli.Command{
				{
					Name:        "create",
					ArgsUsage:   "<user ID or email>",
					Usage:       "Creates a new userkey",
					Description: "$> userkey create bob\n   $> user create --name=mykey bob",
					Flags: []cli.Flag{
						cli.StringFlag{Name: "comment"},
					},
					Action: func(c *cli.Context) error {
						if c.NArg() != 1 {
							return cli.ShowSubcommandHelp(c)
						}

						user, err := FindUserByIdOrEmail(db, c.Args().First())
						if err != nil {
							return err
						}

						fmt.Fprintf(s, "Enter key:\n")
						reader := bufio.NewReader(s)
						text, _ := reader.ReadString('\n')
						fmt.Println(text)

						key, comment, _, _, err := ssh.ParseAuthorizedKey([]byte(text))
						if err != nil {
							return err
						}

						userkey := UserKey{
							UserID:  user.ID,
							Key:     key.Marshal(),
							Comment: comment,
						}
						if c.String("comment") != "" {
							userkey.Comment = c.String("comment")
						}

						// save the userkey in database
						if err := db.Create(&userkey).Error; err != nil {
							return err
						}
						fmt.Fprintf(s, "%d\n", userkey.ID)
						return nil
					},
				}, {
					Name:      "inspect",
					Usage:     "Shows detailed information on one or more userkeys",
					ArgsUsage: "<id> [<id> [<id>...]]",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						userkeys, err := FindUserkeysById(db, c.Args())
						if err != nil {
							return nil
						}

						enc := json.NewEncoder(s)
						enc.SetIndent("", "  ")
						return enc.Encode(userkeys)
					},
				}, {
					Name:  "ls",
					Usage: "Lists userkeys",
					Action: func(c *cli.Context) error {
						var userkeys []UserKey
						if err := db.Preload("User").Find(&userkeys).Error; err != nil {
							return err
						}
						table := tablewriter.NewWriter(s)
						table.SetHeader([]string{"ID", "User", "Comment"})
						table.SetBorder(false)
						table.SetCaption(true, fmt.Sprintf("Total: %d userkeys.", len(userkeys)))
						for _, userkey := range userkeys {
							table.Append([]string{
								fmt.Sprintf("%d", userkey.ID),
								userkey.User.Email,
								// FIXME: add fingerprint
								userkey.Comment,
							})
						}
						table.Render()
						return nil
					},
				}, {
					Name:      "rm",
					Usage:     "Removes one or more userkeys",
					ArgsUsage: "<id> [<id> [<id>...]]",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						userkeys, err := FindUserkeysById(db, c.Args())
						if err != nil {
							return nil
						}

						for _, userkey := range userkeys {
							db.Where("id = ?", userkey.ID).Delete(&UserKey{})
							fmt.Fprintf(s, "%d\n", userkey.ID)
						}
						return nil
					},
				},
			},
		}, {
			Name:  "version",
			Usage: "Shows the SSHPortal version information",
			Action: func(c *cli.Context) error {
				fmt.Fprintf(s, "%s\n", version)
				return nil
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
