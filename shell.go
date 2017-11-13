package main

import (
	"bufio"
	"encoding/json"
	"errors"
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
		return
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
			Name:  "acl",
			Usage: "Manages acls",
			Subcommands: []cli.Command{
				{
					Name:        "create",
					Usage:       "Creates a new ACL",
					Description: "$> acl create -",
					Flags: []cli.Flag{
						cli.StringSliceFlag{Name: "hostgroup, hg", Usage: "Assigns host groups to the acl"},
						cli.StringSliceFlag{Name: "usergroup, ug", Usage: "Assigns host groups to the acl"},
						cli.StringFlag{Name: "pattern", Usage: "Assigns a host pattern to the acl"},
						cli.StringFlag{Name: "comment"},
						cli.StringFlag{Name: "action", Usage: "Assigns the ACL action (allow,deny)", Value: "allow"},
						cli.UintFlag{Name: "weight, w", Usage: "Assigns the ACL weight (priority)"},
					},
					Action: func(c *cli.Context) error {
						acl := ACL{
							Comment:     c.String("comment"),
							HostPattern: c.String("pattern"),
							UserGroups:  []UserGroup{},
							HostGroups:  []HostGroup{},
							Weight:      c.Uint("weight"),
							Action:      c.String("action"),
						}
						if acl.Action != "allow" && acl.Action != "deny" {
							return fmt.Errorf("invalid action %q, allowed values: allow, deny", acl.Action)
						}

						for _, name := range c.StringSlice("usergroup") {
							userGroup, err := FindUserGroupByIdOrName(db, name)
							if err != nil {
								return fmt.Errorf("unknown user group %q: %v", name, err)
							}
							acl.UserGroups = append(acl.UserGroups, *userGroup)
						}
						for _, name := range c.StringSlice("hostgroup") {
							hostGroup, err := FindHostGroupByIdOrName(db, name)
							if err != nil {
								return fmt.Errorf("unknown host group %q: %v", name, err)
							}
							acl.HostGroups = append(acl.HostGroups, *hostGroup)
						}

						if len(acl.UserGroups) == 0 {
							return fmt.Errorf("an ACL must have at least one user group")
						}
						if len(acl.HostGroups) == 0 && acl.HostPattern == "" {
							return fmt.Errorf("an ACL must have at least one host group or host pattern")
						}

						if err := db.Create(&acl).Error; err != nil {
							return err
						}
						fmt.Fprintf(s, "%d\n", acl.ID)
						return nil
					},
				}, {
					Name:      "inspect",
					Usage:     "Shows detailed information on one or more acls",
					ArgsUsage: "<id> [<id> [<id>...]]",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						acls, err := FindACLsById(db, c.Args())
						if err != nil {
							return nil
						}

						enc := json.NewEncoder(s)
						enc.SetIndent("", "  ")
						return enc.Encode(acls)
					},
				}, {
					Name:  "ls",
					Usage: "Lists acls",
					Action: func(c *cli.Context) error {
						var acls []ACL
						if err := db.Preload("UserGroups").Preload("HostGroups").Find(&acls).Error; err != nil {
							return err
						}
						table := tablewriter.NewWriter(s)
						table.SetHeader([]string{"ID", "User groups", "Host groups", "Host pattern", "Action", "Comment"})
						table.SetBorder(false)
						table.SetCaption(true, fmt.Sprintf("Total: %d acls.", len(acls)))
						for _, acl := range acls {
							userGroups := []string{}
							hostGroups := []string{}
							for _, entity := range acl.UserGroups {
								userGroups = append(userGroups, entity.Name)
							}
							for _, entity := range acl.HostGroups {
								hostGroups = append(hostGroups, entity.Name)
							}

							table.Append([]string{
								fmt.Sprintf("%d", acl.ID),
								strings.Join(userGroups, ", "),
								strings.Join(hostGroups, ", "),
								acl.HostPattern,
								acl.Action,
								acl.Comment,
							})
						}
						table.Render()
						return nil
					},
				}, {
					Name:      "rm",
					Usage:     "Removes one or more acls",
					ArgsUsage: "<id or name> [<id or name> [<id or name>...]]",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						acls, err := FindACLsById(db, c.Args())
						if err != nil {
							return nil
						}

						for _, acl := range acls {
							db.Where("id = ?", acl.ID).Delete(&ACL{})
							fmt.Fprintf(s, "%d\n", acl.ID)
						}
						return nil
					},
				},
			},
		}, {
			Name:  "config",
			Usage: "Manages global configuration",
			Subcommands: []cli.Command{
				{
					Name:  "backup",
					Usage: "Dumps a backup",
					Flags: []cli.Flag{
						cli.BoolFlag{Name: "indent", Usage: "uses indented JSON"},
					},
					Description: "ssh admin@portal config backup > sshportal.bkp",
					Action: func(c *cli.Context) error {
						config := Config{}
						if err := db.Find(&config.Hosts).Error; err != nil {
							return err
						}
						if err := db.Find(&config.SSHKeys).Error; err != nil {
							return err
						}
						if err := db.Find(&config.Hosts).Error; err != nil {
							return err
						}
						if err := db.Find(&config.UserKeys).Error; err != nil {
							return err
						}
						if err := db.Find(&config.Users).Error; err != nil {
							return err
						}
						if err := db.Find(&config.UserGroups).Error; err != nil {
							return err
						}
						if err := db.Find(&config.HostGroups).Error; err != nil {
							return err
						}
						if err := db.Find(&config.ACLs).Error; err != nil {
							return err
						}
						config.Date = time.Now()
						enc := json.NewEncoder(s)
						if c.Bool("indent") {
							enc.SetIndent("", "  ")
						}
						return enc.Encode(config)
					},
				}, {
					Name:        "restore",
					Usage:       "Restores a backup",
					Description: "ssh admin@portal config restore < sshportal.bkp",
					Flags: []cli.Flag{
						cli.BoolFlag{Name: "confirm", Usage: "automatically confirms"},
					},
					Action: func(c *cli.Context) error {
						config := Config{}

						dec := json.NewDecoder(s)
						if err := dec.Decode(&config); err != nil {
							return err
						}

						fmt.Fprintf(s, "Loaded backup file (date=%v)\n", config.Date)
						fmt.Fprintf(s, "* %d ACLs\n", len(config.ACLs))
						fmt.Fprintf(s, "* %d HostGroups\n", len(config.HostGroups))
						fmt.Fprintf(s, "* %d Hosts\n", len(config.Hosts))
						fmt.Fprintf(s, "* %d Keys\n", len(config.SSHKeys))
						fmt.Fprintf(s, "* %d UserGroups\n", len(config.UserGroups))
						fmt.Fprintf(s, "* %d Userkeys\n", len(config.UserKeys))
						fmt.Fprintf(s, "* %d Users\n", len(config.Users))

						if !c.Bool("confirm") {
							fmt.Fprintf(s, "restore will erase and replace everything in the database.\nIf you are ok, add the '--confirm' to the restore command\n")
							return errors.New("")
						}

						tx := db.Begin()

						// FIXME: do everything in a transaction
						for _, tableName := range []string{"hosts", "users", "acls", "host_groups", "user_groups", "ssh_keys", "user_keys"} {
							if err := tx.Exec(fmt.Sprintf("DELETE FROM %s;", tableName)).Error; err != nil {
								tx.Rollback()
								return err
							}
						}
						for _, host := range config.Hosts {
							if err := tx.Create(&host).Error; err != nil {
								tx.Rollback()
								return err
							}
						}
						for _, user := range config.Users {
							if err := tx.Create(&user).Error; err != nil {
								tx.Rollback()
								return err
							}
						}
						for _, acl := range config.ACLs {
							if err := tx.Create(&acl).Error; err != nil {
								tx.Rollback()
								return err
							}
						}
						for _, hostGroup := range config.HostGroups {
							if err := tx.Create(&hostGroup).Error; err != nil {
								tx.Rollback()
								return err
							}
						}
						for _, userGroup := range config.UserGroups {
							if err := tx.Create(&userGroup).Error; err != nil {
								tx.Rollback()
								return err
							}
						}
						for _, sshKey := range config.SSHKeys {
							if err := tx.Create(&sshKey).Error; err != nil {
								tx.Rollback()
								return err
							}
						}
						for _, userKey := range config.UserKeys {
							if err := tx.Create(&userKey).Error; err != nil {
								tx.Rollback()
								return err
							}
						}

						if err := tx.Commit().Error; err != nil {
							return err
						}

						fmt.Fprintf(s, "Import done.\n")
						return nil
					},
				},
			},
		}, {
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
					ArgsUsage: "<id or name> [<id or name> [<id or name>...]]",
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
					ArgsUsage: "<id or name> [<id or name> [<id or name>...]]",
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
					ArgsUsage: "<id or name> [<id or name> [<id or name>...]]",
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
						if err := db.Preload("ACLs").Preload("Hosts").Find(&hostGroups).Error; err != nil {
							return err
						}
						table := tablewriter.NewWriter(s)
						table.SetHeader([]string{"ID", "Name", "Hosts", "ACLs", "Comment"})
						table.SetBorder(false)
						table.SetCaption(true, fmt.Sprintf("Total: %d host groups.", len(hostGroups)))
						for _, hostGroup := range hostGroups {
							// FIXME: add more stats (amount of hosts, linked usergroups, ...)
							table.Append([]string{
								fmt.Sprintf("%d", hostGroup.ID),
								hostGroup.Name,
								fmt.Sprintf("%d", len(hostGroup.Hosts)),
								fmt.Sprintf("%d", len(hostGroup.ACLs)),
								hostGroup.Comment,
							})
						}
						table.Render()
						return nil
					},
				}, {
					Name:      "rm",
					Usage:     "Removes one or more host groups",
					ArgsUsage: "<id or name> [<id or name> [<id or name>...]]",
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
				fmt.Fprintf(s, "Version: %s\n", VERSION)
				fmt.Fprintf(s, "GIT SHA: %s\n", GIT_SHA)
				fmt.Fprintf(s, "GIT Branch: %s\n", GIT_BRANCH)
				fmt.Fprintf(s, "GIT Tag: %s\n", GIT_TAG)

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
					ArgsUsage: "<id or name> [<id or name> [<id or name>...]]",
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
					ArgsUsage: "<id or name> [<id or name> [<id or name>...]]",
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
					ArgsUsage: "<id or email> [<id or email> [<id or email>...]]",
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
					ArgsUsage: "<id or email> [<id or email> [<id or email>...]]",
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
					ArgsUsage: "<id or name> [<id or name> [<id or name>...]]",
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
						if err := db.Preload("ACLs").Preload("Users").Find(&userGroups).Error; err != nil {
							return err
						}
						table := tablewriter.NewWriter(s)
						table.SetHeader([]string{"ID", "Name", "Users", "ACLs", "Comment"})
						table.SetBorder(false)
						table.SetCaption(true, fmt.Sprintf("Total: %d user groups.", len(userGroups)))
						for _, userGroup := range userGroups {
							// FIXME: add more stats (amount of users, linked usergroups, ...)
							table.Append([]string{
								fmt.Sprintf("%d", userGroup.ID),
								userGroup.Name,
								fmt.Sprintf("%d", len(userGroup.Users)),
								fmt.Sprintf("%d", len(userGroup.ACLs)),
								userGroup.Comment,
							})
						}
						table.Render()
						return nil
					},
				}, {
					Name:      "rm",
					Usage:     "Removes one or more user groups",
					ArgsUsage: "<id or name> [<id or name> [<id or name>...]]",
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
				fmt.Fprintf(s, "%s\n", VERSION)
				return nil
			},
		}, {
			Name:  "exit",
			Usage: "Exit",
			Action: func(c *cli.Context) error {
				return cli.NewExitError("", 0)
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
			if len(words) == 1 && strings.ToLower(words[0]) == "exit" {
				s.Exit(0)
				return nil
			}
			if err := app.Run(append([]string{"config"}, words...)); err != nil {
				if cliErr, ok := err.(*cli.ExitError); ok {
					if cliErr.ExitCode() != 0 {
						io.WriteString(s, fmt.Sprintf("error: %v\n", err))
					}
					//s.Exit(cliErr.ExitCode())
				} else {
					io.WriteString(s, fmt.Sprintf("error: %v\n", err))
				}
			}
		}
	} else { // oneshot mode
		if err := app.Run(append([]string{"config"}, sshCommand...)); err != nil {
			if errMsg := err.Error(); errMsg != "" {
				io.WriteString(s, fmt.Sprintf("error: %s\n", errMsg))
			}
			if cliErr, ok := err.(*cli.ExitError); ok {
				s.Exit(cliErr.ExitCode())
			} else {
				s.Exit(1)
			}
		}
	}

	return nil
}
