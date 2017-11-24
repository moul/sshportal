package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	shlex "github.com/anmitsu/go-shlex"
	"github.com/asaskevich/govalidator"
	humanize "github.com/dustin/go-humanize"
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

	myself := s.Context().Value(userContextKey).(User)
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
						cli.StringSliceFlag{Name: "hostgroup, hg", Usage: "Assigns `HOSTGROUPS` to the acl"},
						cli.StringSliceFlag{Name: "usergroup, ug", Usage: "Assigns `HOSTGROUPS` to the acl"},
						cli.StringFlag{Name: "pattern", Usage: "Assigns a host pattern to the acl"},
						cli.StringFlag{Name: "comment", Usage: "Adds a comment"},
						cli.StringFlag{Name: "action", Usage: "Assigns the ACL action (allow,deny)", Value: "allow"},
						cli.UintFlag{Name: "weight, w", Usage: "Assigns the ACL weight (priority)"},
					},
					Action: func(c *cli.Context) error {
						if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
							return err
						}
						acl := ACL{
							Comment:     c.String("comment"),
							HostPattern: c.String("pattern"),
							UserGroups:  []*UserGroup{},
							HostGroups:  []*HostGroup{},
							Weight:      c.Uint("weight"),
							Action:      c.String("action"),
						}
						if acl.Action != "allow" && acl.Action != "deny" {
							return fmt.Errorf("invalid action %q, allowed values: allow, deny", acl.Action)
						}
						if _, err := govalidator.ValidateStruct(acl); err != nil {
							return err
						}

						var userGroups []*UserGroup
						if err := UserGroupsPreload(UserGroupsByIdentifiers(db, c.StringSlice("usergroup"))).Find(&userGroups).Error; err != nil {
							return err
						}
						acl.UserGroups = append(acl.UserGroups, userGroups...)
						var hostGroups []*HostGroup
						if err := HostGroupsPreload(HostGroupsByIdentifiers(db, c.StringSlice("hostgroup"))).Find(&hostGroups).Error; err != nil {
							return err
						}
						acl.HostGroups = append(acl.HostGroups, hostGroups...)

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
					ArgsUsage: "ACL...",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}
						if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
							return err
						}

						var acls []ACL
						if err := ACLsPreload(ACLsByIdentifiers(db, c.Args())).Find(&acls).Error; err != nil {
							return err
						}

						enc := json.NewEncoder(s)
						enc.SetIndent("", "  ")
						return enc.Encode(acls)
					},
				}, {
					Name:  "ls",
					Usage: "Lists acls",
					Action: func(c *cli.Context) error {
						if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
							return err
						}
						var acls []ACL
						if err := db.Order("created_at desc").Preload("UserGroups").Preload("HostGroups").Find(&acls).Error; err != nil {
							return err
						}
						table := tablewriter.NewWriter(s)
						table.SetHeader([]string{"ID", "Weight", "User groups", "Host groups", "Host pattern", "Action", "Updated", "Created", "Comment"})
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
								fmt.Sprintf("%d", acl.Weight),
								strings.Join(userGroups, ", "),
								strings.Join(hostGroups, ", "),
								acl.HostPattern,
								acl.Action,
								humanize.Time(acl.UpdatedAt),
								humanize.Time(acl.CreatedAt),
								acl.Comment,
							})
						}
						table.Render()
						return nil
					},
				}, {
					Name:      "rm",
					Usage:     "Removes one or more acls",
					ArgsUsage: "ACL...",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}
						if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
							return err
						}

						return ACLsByIdentifiers(db, c.Args()).Delete(&ACL{}).Error
					},
				}, {
					Name:      "update",
					Usage:     "Updates an existing acl",
					ArgsUsage: "ACL...",
					Flags: []cli.Flag{
						cli.StringFlag{Name: "action, a", Usage: "Update action"},
						cli.StringFlag{Name: "pattern, p", Usage: "Update host-pattern"},
						cli.UintFlag{Name: "weight, w", Usage: "Update weight"},
						cli.StringFlag{Name: "comment, c", Usage: "Update comment"},
						cli.StringSliceFlag{Name: "assign-usergroup, ug", Usage: "Assign the ACL to new `USERGROUPS`"},
						cli.StringSliceFlag{Name: "unassign-usergroup", Usage: "Unassign the ACL from `USERGROUPS`"},
						cli.StringSliceFlag{Name: "assign-hostgroup, hg", Usage: "Assign the ACL to new `HOSTGROUPS`"},
						cli.StringSliceFlag{Name: "unassign-hostgroup", Usage: "Unassign the ACL from `HOSTGROUPS`"},
					},
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}
						if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
							return err
						}

						var acls []ACL
						if err := ACLsByIdentifiers(db, c.Args()).Find(&acls).Error; err != nil {
							return err
						}

						tx := db.Begin()
						for _, acl := range acls {
							model := tx.Model(&acl)
							update := ACL{
								Action:      c.String("action"),
								HostPattern: c.String("pattern"),
								Weight:      c.Uint("weight"),
								Comment:     c.String("comment"),
							}
							if err := model.Updates(update).Error; err != nil {
								tx.Rollback()
								return err
							}

							// associations
							var appendUserGroups []UserGroup
							var deleteUserGroups []UserGroup
							if err := UserGroupsByIdentifiers(db, c.StringSlice("assign-usergroup")).Find(&appendUserGroups).Error; err != nil {
								tx.Rollback()
								return err
							}
							if err := UserGroupsByIdentifiers(db, c.StringSlice("unassign-usergroup")).Find(&deleteUserGroups).Error; err != nil {
								tx.Rollback()
								return err
							}
							if err := model.Association("UserGroups").Append(&appendUserGroups).Delete(deleteUserGroups).Error; err != nil {
								tx.Rollback()
								return err
							}

							var appendHostGroups []HostGroup
							var deleteHostGroups []HostGroup
							if err := HostGroupsByIdentifiers(db, c.StringSlice("assign-hostgroup")).Find(&appendHostGroups).Error; err != nil {
								tx.Rollback()
								return err
							}
							if err := HostGroupsByIdentifiers(db, c.StringSlice("unassign-hostgroup")).Find(&deleteHostGroups).Error; err != nil {
								tx.Rollback()
								return err
							}
							if err := model.Association("HostGroups").Append(&appendHostGroups).Delete(deleteHostGroups).Error; err != nil {
								tx.Rollback()
								return err
							}
						}

						return tx.Commit().Error
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
						if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
							return err
						}

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
						if err := db.Find(&config.Settings).Error; err != nil {
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
						cli.BoolFlag{Name: "confirm", Usage: "yes, I want to replace everything with this backup!"},
					},
					Action: func(c *cli.Context) error {
						if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
							return err
						}

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
						fmt.Fprintf(s, "* %d Settings\n", len(config.Settings))

						if !c.Bool("confirm") {
							fmt.Fprintf(s, "restore will erase and replace everything in the database.\nIf you are ok, add the '--confirm' to the restore command\n")
							return errors.New("")
						}

						tx := db.Begin()

						// FIXME: do everything in a transaction
						for _, tableName := range []string{"hosts", "users", "acls", "host_groups", "user_groups", "ssh_keys", "user_keys", "settings"} {
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
						for _, setting := range config.Settings {
							if err := tx.Create(&setting).Error; err != nil {
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
						cli.StringFlag{Name: "name, n", Usage: "Assigns a name to the host"},
						cli.StringFlag{Name: "password, p", Usage: "If present, sshportal will use password-based authentication"},
						cli.StringFlag{Name: "fingerprint, f", Usage: "SSH host key fingerprint"},
						cli.StringFlag{Name: "comment, c"},
						cli.StringFlag{Name: "key, k", Usage: "`KEY` to use for authentication"},
						cli.StringSliceFlag{Name: "group, g", Usage: "Assigns the host to `HOSTGROUPS` (default: \"default\")"},
					},
					Action: func(c *cli.Context) error {
						if c.NArg() != 1 {
							return cli.ShowSubcommandHelp(c)
						}

						if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
							return err
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
						// FIXME: check if name already exists
						host.Comment = c.String("comment")

						if _, err := govalidator.ValidateStruct(host); err != nil {
							return err
						}

						inputKey := c.String("key")
						if inputKey == "" && host.Password == "" {
							inputKey = "default"
						}
						if inputKey != "" {
							var key SSHKey
							if err := SSHKeysByIdentifiers(db, []string{inputKey}).First(&key).Error; err != nil {
								return err
							}
							host.SSHKeyID = key.ID
						}

						// host group
						inputGroups := c.StringSlice("group")
						if len(inputGroups) == 0 {
							inputGroups = []string{"default"}
						}
						if err := HostGroupsByIdentifiers(db, inputGroups).Find(&host.Groups).Error; err != nil {
							return err
						}

						if err := db.Create(&host).Error; err != nil {
							return err
						}
						fmt.Fprintf(s, "%d\n", host.ID)
						return nil
					},
				}, {
					Name:      "inspect",
					Usage:     "Shows detailed information on one or more hosts",
					ArgsUsage: "HOST...",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						if err := UserCheckRoles(myself, []string{"admin", "listhosts"}); err != nil {
							return err
						}

						var hosts []Host
						db = db.Preload("Groups")
						if UserHasRole(myself, "admin") {
							db = db.Preload("SSHKey")
						}
						if err := HostsByIdentifiers(db, c.Args()).Find(&hosts).Error; err != nil {
							return err
						}

						enc := json.NewEncoder(s)
						enc.SetIndent("", "  ")
						return enc.Encode(hosts)
					},
				}, {
					Name:  "ls",
					Usage: "Lists hosts",
					Action: func(c *cli.Context) error {
						if err := UserCheckRoles(myself, []string{"admin", "listhosts"}); err != nil {
							return err
						}

						var hosts []*Host
						if err := db.Order("created_at desc").Preload("Groups").Find(&hosts).Error; err != nil {
							return err
						}
						table := tablewriter.NewWriter(s)
						table.SetHeader([]string{"ID", "Name", "URL", "Key", "Pass", "Groups", "Updated", "Created", "Comment"})
						table.SetBorder(false)
						table.SetCaption(true, fmt.Sprintf("Total: %d hosts.", len(hosts)))
						for _, host := range hosts {
							authKey, authPass := "", ""
							if host.Password != "" {
								authPass = "yes"
							}
							if host.SSHKeyID > 0 {
								var key SSHKey
								db.Model(&host).Related(&key)
								authKey = key.Name
							}
							groupNames := []string{}
							for _, hostGroup := range host.Groups {
								groupNames = append(groupNames, hostGroup.Name)
							}
							table.Append([]string{
								fmt.Sprintf("%d", host.ID),
								host.Name,
								host.URL(),
								authKey,
								authPass,
								strings.Join(groupNames, ", "),
								humanize.Time(host.UpdatedAt),
								humanize.Time(host.CreatedAt),
								host.Comment,
								//FIXME: add some stats about last access time etc
							})
						}
						table.Render()
						return nil
					},
				}, {
					Name:      "rm",
					Usage:     "Removes one or more hosts",
					ArgsUsage: "HOST...",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
							return err
						}

						return HostsByIdentifiers(db, c.Args()).Delete(&Host{}).Error
					},
				}, {
					Name:      "update",
					Usage:     "Updates an existing host",
					ArgsUsage: "HOST...",
					Flags: []cli.Flag{
						cli.StringFlag{Name: "name, n", Usage: "Rename the host"},
						cli.StringFlag{Name: "password, p", Usage: "Update/set a password, use \"none\" to unset"},
						cli.StringFlag{Name: "fingerprint, f", Usage: "Update/set a host fingerprint, use \"none\" to unset"},
						cli.StringFlag{Name: "comment, c", Usage: "Update/set a host comment"},
						cli.StringFlag{Name: "key, k", Usage: "Link a `KEY` to use for authentication"},
						cli.StringSliceFlag{Name: "assign-group, g", Usage: "Assign the host to a new `HOSTGROUPS`"},
						cli.StringSliceFlag{Name: "unassign-group", Usage: "Unassign the host from a `HOSTGROUPS`"},
					},
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
							return err
						}

						var hosts []Host
						if err := HostsByIdentifiers(db, c.Args()).Find(&hosts).Error; err != nil {
							return err
						}

						if len(hosts) > 1 && c.String("name") != "" {
							return fmt.Errorf("cannot set --name when editing multiple hosts at once")
						}

						tx := db.Begin()
						for _, host := range hosts {
							model := tx.Model(&host)
							// simple fields
							for _, fieldname := range []string{"name", "comment", "password", "fingerprint"} {
								if c.String(fieldname) != "" {
									if err := model.Update(fieldname, c.String(fieldname)).Error; err != nil {
										tx.Rollback()
										return err
									}
								}
							}

							// associations
							if c.String("key") != "" {
								var key SSHKey
								if err := SSHKeysByIdentifiers(db, []string{c.String("key")}).First(&key).Error; err != nil {
									tx.Rollback()
									return err
								}
								if err := model.Association("SSHKey").Replace(&key).Error; err != nil {
									tx.Rollback()
									return err
								}
							}
							var appendGroups []HostGroup
							var deleteGroups []HostGroup
							if err := HostGroupsByIdentifiers(db, c.StringSlice("assign-group")).Find(&appendGroups).Error; err != nil {
								tx.Rollback()
								return err
							}
							if err := HostGroupsByIdentifiers(db, c.StringSlice("unassign-group")).Find(&deleteGroups).Error; err != nil {
								tx.Rollback()
								return err
							}
							if err := model.Association("Groups").Append(&appendGroups).Delete(deleteGroups).Error; err != nil {
								tx.Rollback()
								return err
							}
						}

						return tx.Commit().Error
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
						cli.StringFlag{Name: "comment", Usage: "Adds a comment"},
					},
					Action: func(c *cli.Context) error {
						if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
							return err
						}

						hostGroup := HostGroup{
							Name:    c.String("name"),
							Comment: c.String("comment"),
						}
						if hostGroup.Name == "" {
							hostGroup.Name = namesgenerator.GetRandomName(0)
						}
						if _, err := govalidator.ValidateStruct(hostGroup); err != nil {
							return err
						}
						// FIXME: check if name already exists

						if err := db.Create(&hostGroup).Error; err != nil {
							return err
						}
						fmt.Fprintf(s, "%d\n", hostGroup.ID)
						return nil
					},
				}, {
					Name:      "inspect",
					Usage:     "Shows detailed information on one or more host groups",
					ArgsUsage: "HOSTGROUP...",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
							return err
						}

						var hostGroups []HostGroup
						if err := HostGroupsPreload(HostGroupsByIdentifiers(db, c.Args())).Find(&hostGroups).Error; err != nil {
							return err
						}

						enc := json.NewEncoder(s)
						enc.SetIndent("", "  ")
						return enc.Encode(hostGroups)
					},
				}, {
					Name:  "ls",
					Usage: "Lists host groups",
					Action: func(c *cli.Context) error {
						if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
							return err
						}

						var hostGroups []*HostGroup
						if err := db.Order("created_at desc").Preload("ACLs").Preload("Hosts").Find(&hostGroups).Error; err != nil {
							return err
						}
						table := tablewriter.NewWriter(s)
						table.SetHeader([]string{"ID", "Name", "Hosts", "ACLs", "Updated", "Created", "Comment"})
						table.SetBorder(false)
						table.SetCaption(true, fmt.Sprintf("Total: %d host groups.", len(hostGroups)))
						for _, hostGroup := range hostGroups {
							// FIXME: add more stats (amount of hosts, linked usergroups, ...)
							table.Append([]string{
								fmt.Sprintf("%d", hostGroup.ID),
								hostGroup.Name,
								fmt.Sprintf("%d", len(hostGroup.Hosts)),
								fmt.Sprintf("%d", len(hostGroup.ACLs)),
								humanize.Time(hostGroup.UpdatedAt),
								humanize.Time(hostGroup.CreatedAt),
								hostGroup.Comment,
							})
						}
						table.Render()
						return nil
					},
				}, {
					Name:      "rm",
					Usage:     "Removes one or more host groups",
					ArgsUsage: "HOSTGROUP...",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
							return err
						}

						return HostGroupsByIdentifiers(db, c.Args()).Delete(&HostGroup{}).Error
					},
				},
			},
		}, {
			Name:  "info",
			Usage: "Shows system-wide information",
			Action: func(c *cli.Context) error {
				if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
					return err
				}

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
						cli.StringFlag{Name: "comment", Usage: "Adds a comment"},
					},
					Action: func(c *cli.Context) error {
						if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
							return err
						}

						name := namesgenerator.GetRandomName(0)
						if c.String("name") != "" {
							name = c.String("name")
						}

						key, err := NewSSHKey(c.String("type"), c.Uint("length"))
						if err != nil {
							return err
						}
						key.Name = name
						key.Comment = c.String("comment")

						if _, err := govalidator.ValidateStruct(key); err != nil {
							return err
						}
						// FIXME: check if name already exists

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
					ArgsUsage: "KEY...",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
							return err
						}

						var keys []SSHKey
						if err := SSHKeysByIdentifiers(db, c.Args()).Find(&keys).Error; err != nil {
							return err
						}

						enc := json.NewEncoder(s)
						enc.SetIndent("", "  ")
						return enc.Encode(keys)
					},
				}, {
					Name:  "ls",
					Usage: "Lists keys",
					Action: func(c *cli.Context) error {
						if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
							return err
						}

						var keys []SSHKey
						if err := db.Order("created_at desc").Preload("Hosts").Find(&keys).Error; err != nil {
							return err
						}
						table := tablewriter.NewWriter(s)
						table.SetHeader([]string{"ID", "Name", "Type", "Length", "Hosts", "Updated", "Created", "Comment"})
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
								humanize.Time(key.UpdatedAt),
								humanize.Time(key.CreatedAt),
								key.Comment,
								//FIXME: add some stats
							})
						}
						table.Render()
						return nil
					},
				}, {
					Name:      "rm",
					Usage:     "Removes one or more keys",
					ArgsUsage: "KEY...",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
							return err
						}

						return SSHKeysByIdentifiers(db, c.Args()).Delete(&SSHKey{}).Error
					},
				}, {
					Name:      "setup",
					Usage:     "Return shell command to install key on remote host",
					ArgsUsage: "KEY",
					Action: func(c *cli.Context) error {
						if c.NArg() != 1 {
							return cli.ShowSubcommandHelp(c)
						}

						// not checking roles, everyone with an account can see how to enroll new hosts

						var key SSHKey
						if err := SSHKeysByIdentifiers(db, c.Args()).First(&key).Error; err != nil {
							return err
						}
						fmt.Fprintf(s, "umask 077; mkdir -p .ssh; echo %s sshportal >> .ssh/authorized_keys\n", key.PubKey)
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
					ArgsUsage: "USER...",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
							return err
						}

						var users []User
						if err := UsersPreload(UsersByIdentifiers(db, c.Args())).Find(&users).Error; err != nil {
							return err
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
						cli.StringFlag{Name: "comment", Usage: "Adds a comment"},
						cli.StringSliceFlag{Name: "group, g", Usage: "Names or IDs of `USERGROUPS` (default: \"default\")"},
					},
					Action: func(c *cli.Context) error {
						if c.NArg() != 1 {
							return cli.ShowSubcommandHelp(c)
						}

						if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
							return err
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

						if _, err := govalidator.ValidateStruct(user); err != nil {
							return err
						}

						// user group
						inputGroups := c.StringSlice("group")
						if len(inputGroups) == 0 {
							inputGroups = []string{"default"}
						}
						if err := UserGroupsByIdentifiers(db, inputGroups).Find(&user.Groups).Error; err != nil {
							return err
						}

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
						if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
							return err
						}

						var users []User
						if err := db.Order("created_at desc").Preload("Groups").Preload("Roles").Preload("Keys").Find(&users).Error; err != nil {
							return err
						}
						table := tablewriter.NewWriter(s)
						table.SetHeader([]string{"ID", "Name", "Email", "Roles", "Keys", "Groups", "Updated", "Created", "Comment"})
						table.SetBorder(false)
						table.SetCaption(true, fmt.Sprintf("Total: %d users.", len(users)))
						for _, user := range users {
							groupNames := []string{}
							for _, userGroup := range user.Groups {
								groupNames = append(groupNames, userGroup.Name)
							}
							roleNames := []string{}
							for _, role := range user.Roles {
								roleNames = append(roleNames, role.Name)
							}
							table.Append([]string{
								fmt.Sprintf("%d", user.ID),
								user.Name,
								user.Email,
								strings.Join(roleNames, ", "),
								fmt.Sprintf("%d", len(user.Keys)),
								strings.Join(groupNames, ", "),
								humanize.Time(user.UpdatedAt),
								humanize.Time(user.CreatedAt),
								user.Comment,
							})
						}
						table.Render()
						return nil
					},
				}, {
					Name:      "rm",
					Usage:     "Removes one or more users",
					ArgsUsage: "USER...",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
							return err
						}

						return UsersByIdentifiers(db, c.Args()).Delete(&User{}).Error
					},
				}, {
					Name:      "update",
					Usage:     "Updates an existing user",
					ArgsUsage: "USER...",
					Flags: []cli.Flag{
						cli.StringFlag{Name: "name, n", Usage: "Renames the user"},
						cli.StringFlag{Name: "email, e", Usage: "Updates the email"},
						cli.StringSliceFlag{Name: "assign-role, r", Usage: "Assign the user to new `USERROLES`"},
						cli.StringSliceFlag{Name: "unassign-role", Usage: "Unassign the user from `USERROLES`"},
						cli.StringSliceFlag{Name: "assign-group, g", Usage: "Assign the user to new `USERGROUPS`"},
						cli.StringSliceFlag{Name: "unassign-group", Usage: "Unassign the user from `USERGROUPS`"},
					},
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
							return err
						}

						// FIXME: check if unset-admin + user == myself
						var users []User
						if err := UsersByIdentifiers(db, c.Args()).Find(&users).Error; err != nil {
							return err
						}

						if c.Bool("set-admin") && c.Bool("unset-admin") {
							return fmt.Errorf("cannot use --set-admin and --unset-admin altogether")
						}

						if len(users) > 1 && c.String("email") != "" {
							return fmt.Errorf("cannot set --email when editing multiple users at once")
						}

						tx := db.Begin()
						for _, user := range users {
							model := tx.Model(&user)
							// simple fields
							for _, fieldname := range []string{"name", "email", "comment"} {
								if c.String(fieldname) != "" {
									if err := model.Update(fieldname, c.String(fieldname)).Error; err != nil {
										tx.Rollback()
										return err
									}
								}
							}

							// associations
							var appendGroups []UserGroup
							if err := UserGroupsByIdentifiers(db, c.StringSlice("assign-group")).Find(&appendGroups).Error; err != nil {
								tx.Rollback()
								return err
							}
							var deleteGroups []UserGroup
							if err := UserGroupsByIdentifiers(db, c.StringSlice("unassign-group")).Find(&deleteGroups).Error; err != nil {
								tx.Rollback()
								return err
							}
							if err := model.Association("Groups").Append(&appendGroups).Delete(deleteGroups).Error; err != nil {
								tx.Rollback()
								return err
							}

							var appendRoles []UserRole
							if err := UserRolesByIdentifiers(db, c.StringSlice("assign-role")).Find(&appendRoles).Error; err != nil {
								tx.Rollback()
								return err
							}
							var deleteRoles []UserRole
							if err := UserRolesByIdentifiers(db, c.StringSlice("unassign-role")).Find(&deleteRoles).Error; err != nil {
								tx.Rollback()
								return err
							}
							if err := model.Association("Roles").Append(&appendRoles).Delete(deleteRoles).Error; err != nil {
								tx.Rollback()
								return err
							}
						}

						return tx.Commit().Error
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
						cli.StringFlag{Name: "comment", Usage: "Adds a comment"},
					},
					Action: func(c *cli.Context) error {
						if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
							return err
						}

						userGroup := UserGroup{
							Name:    c.String("name"),
							Comment: c.String("comment"),
						}
						if userGroup.Name == "" {
							userGroup.Name = namesgenerator.GetRandomName(0)
						}

						if _, err := govalidator.ValidateStruct(userGroup); err != nil {
							return err
						}
						// FIXME: check if name already exists
						// FIXME: add myself to the new group

						userGroup.Users = []*User{&myself}

						if err := db.Create(&userGroup).Error; err != nil {
							return err
						}
						fmt.Fprintf(s, "%d\n", userGroup.ID)
						return nil
					},
				}, {
					Name:      "inspect",
					Usage:     "Shows detailed information on one or more user groups",
					ArgsUsage: "USERGROUP...",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
							return err
						}

						var userGroups []UserGroup
						if err := UserGroupsPreload(UserGroupsByIdentifiers(db, c.Args())).Find(&userGroups).Error; err != nil {
							return err
						}

						enc := json.NewEncoder(s)
						enc.SetIndent("", "  ")
						return enc.Encode(userGroups)
					},
				}, {
					Name:  "ls",
					Usage: "Lists user groups",
					Action: func(c *cli.Context) error {
						if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
							return err
						}

						var userGroups []*UserGroup
						if err := db.Order("created_at desc").Preload("ACLs").Preload("Users").Find(&userGroups).Error; err != nil {
							return err
						}
						table := tablewriter.NewWriter(s)
						table.SetHeader([]string{"ID", "Name", "Users", "ACLs", "Update", "Create", "Comment"})
						table.SetBorder(false)
						table.SetCaption(true, fmt.Sprintf("Total: %d user groups.", len(userGroups)))
						for _, userGroup := range userGroups {
							table.Append([]string{
								fmt.Sprintf("%d", userGroup.ID),
								userGroup.Name,
								fmt.Sprintf("%d", len(userGroup.Users)),
								fmt.Sprintf("%d", len(userGroup.ACLs)),
								humanize.Time(userGroup.UpdatedAt),
								humanize.Time(userGroup.CreatedAt),
								userGroup.Comment,
							})
							// FIXME: add more stats (amount of users, linked usergroups, ...)
						}
						table.Render()
						return nil
					},
				}, {
					Name:      "rm",
					Usage:     "Removes one or more user groups",
					ArgsUsage: "USERGROUP...",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
							return err
						}

						return UserGroupsByIdentifiers(db, c.Args()).Delete(&UserGroup{}).Error
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
						cli.StringFlag{Name: "comment", Usage: "Adds a comment"},
					},
					Action: func(c *cli.Context) error {
						if c.NArg() != 1 {
							return cli.ShowSubcommandHelp(c)
						}

						if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
							return err
						}

						var user User
						if err := UsersByIdentifiers(db, c.Args()).First(&user).Error; err != nil {
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
							User:    &user,
							Key:     key.Marshal(),
							Comment: comment,
						}
						if c.String("comment") != "" {
							userkey.Comment = c.String("comment")
						}

						if _, err := govalidator.ValidateStruct(userkey); err != nil {
							return err
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
					ArgsUsage: "USERKEY...",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
							return err
						}

						var userKeys []UserKey
						if err := UserKeysPreload(UserKeysByIdentifiers(db, c.Args())).Find(&userKeys).Error; err != nil {
							return err
						}

						enc := json.NewEncoder(s)
						enc.SetIndent("", "  ")
						return enc.Encode(userKeys)
					},
				}, {
					Name:  "ls",
					Usage: "Lists userkeys",
					Action: func(c *cli.Context) error {
						if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
							return err
						}

						var userkeys []UserKey
						if err := db.Order("created_at desc").Preload("User").Find(&userkeys).Error; err != nil {
							return err
						}
						table := tablewriter.NewWriter(s)
						table.SetHeader([]string{"ID", "User", "Updated", "Created", "Comment"})
						table.SetBorder(false)
						table.SetCaption(true, fmt.Sprintf("Total: %d userkeys.", len(userkeys)))
						for _, userkey := range userkeys {
							table.Append([]string{
								fmt.Sprintf("%d", userkey.ID),
								userkey.User.Email,
								// FIXME: add fingerprint
								humanize.Time(userkey.UpdatedAt),
								humanize.Time(userkey.CreatedAt),
								userkey.Comment,
							})
						}
						table.Render()
						return nil
					},
				}, {
					Name:      "rm",
					Usage:     "Removes one or more userkeys",
					ArgsUsage: "USERKEY...",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						if err := UserCheckRoles(myself, []string{"admin"}); err != nil {
							return err
						}

						return UserKeysByIdentifiers(db, c.Args()).Delete(&UserKey{}).Error
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
