package bastion // import "moul.io/sshportal/pkg/bastion"

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"

	shlex "github.com/anmitsu/go-shlex"
	"github.com/asaskevich/govalidator"
	"github.com/docker/docker/pkg/namesgenerator"
	humanize "github.com/dustin/go-humanize"
	"github.com/gliderlabs/ssh"
	"github.com/mgutz/ansi"
	"github.com/olekukonko/tablewriter"
	"github.com/urfave/cli"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/terminal"
	"moul.io/sshportal/pkg/crypto"
	"moul.io/sshportal/pkg/dbmodels"
)

var banner = `

    __________ _____           __       __
   / __/ __/ // / _ \___  ____/ /____ _/ /
  _\ \_\ \/ _  / ___/ _ \/ __/ __/ _ '/ /
 /___/___/_//_/_/   \___/_/  \__/\_,_/_/


`
var startTime = time.Now()

const (
	naMessage = "n/a"
)

func shell(s ssh.Session, version, gitSha, gitTag string) error {
	var (
		sshCommand = s.Command()
		actx       = s.Context().Value(authContextKey).(*authContext)
	)
	if len(sshCommand) == 0 {
		if _, err := fmt.Fprint(s, banner); err != nil {
			return err
		}
	}

	cli.AppHelpTemplate = `COMMANDS:
{{range .Commands}}{{if not .HideHelp}}   {{join .Names ", "}}{{ "\t"}}{{.Usage}}{{ "\n" }}{{end}}{{end}}{{if .VisibleFlags}}
GLOBAL OPTIONS:
   {{range .VisibleFlags}}{{.}}
   {{end}}{{end}}
`
	cli.OsExiter = func(c int) {}
	cli.HelpFlag = cli.BoolFlag{
		Name:   "help, h",
		Hidden: true,
	}
	app := cli.NewApp()
	app.Writer = s
	app.HideVersion = true

	dbmodels.InitValidator()

	var (
		myself = &actx.user
		db     = actx.db
	)

	app.Commands = []cli.Command{
		{
			Name:  "acl",
			Usage: "Manages ACLs",
			Subcommands: []cli.Command{
				{
					Name:        "create",
					Usage:       "Creates a new ACL",
					Description: "$> acl create -",
					Flags: []cli.Flag{
						cli.StringSliceFlag{Name: "hostgroup, hg", Usage: "Assigns `HOSTGROUPS` to the acl"},
						cli.StringSliceFlag{Name: "usergroup, ug", Usage: "Assigns `USERGROUP` to the acl"},
						cli.StringFlag{Name: "pattern", Usage: "Assigns a host pattern to the acl"},
						cli.StringFlag{Name: "comment", Usage: "Adds a comment"},
						cli.StringFlag{Name: "action", Usage: "Assigns the ACL action (allow,deny)", Value: string(dbmodels.ACLActionAllow)},
						cli.UintFlag{Name: "weight, w", Usage: "Assigns the ACL weight (priority)"},
						cli.StringFlag{Name: "inception, i", Usage: "Assigns inception date-time"},
						cli.StringFlag{Name: "expiration, e", Usage: "Assigns expiration date-time"},
					},
					Action: func(c *cli.Context) error {
						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						inception, err := parseOptionalTime(c.String("inception"))
						if err != nil {
							return err
						}
						expiration, err := parseOptionalTime(c.String("expiration"))
						if err != nil {
							return err
						}

						acl := dbmodels.ACL{
							Comment:     c.String("comment"),
							HostPattern: c.String("pattern"),
							UserGroups:  []*dbmodels.UserGroup{},
							HostGroups:  []*dbmodels.HostGroup{},
							Weight:      c.Uint("weight"),
							Inception:   inception,
							Expiration:  expiration,
							Action:      c.String("action"),
						}
						if acl.Action != string(dbmodels.ACLActionAllow) && acl.Action != string(dbmodels.ACLActionDeny) {
							return fmt.Errorf("invalid action %q, allowed values: allow, deny", acl.Action)
						}
						if _, err := govalidator.ValidateStruct(acl); err != nil {
							return err
						}

						var userGroups []*dbmodels.UserGroup
						if err := dbmodels.UserGroupsPreload(dbmodels.UserGroupsByIdentifiers(db, c.StringSlice("usergroup"))).Find(&userGroups).Error; err != nil {
							return err
						}
						acl.UserGroups = append(acl.UserGroups, userGroups...)
						var hostGroups []*dbmodels.HostGroup
						if err := dbmodels.HostGroupsPreload(dbmodels.HostGroupsByIdentifiers(db, c.StringSlice("hostgroup"))).Find(&hostGroups).Error; err != nil {
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
					Usage:     "Shows detailed information on one or more ACLs",
					ArgsUsage: "ACL...",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}
						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						var acls []dbmodels.ACL
						if err := dbmodels.ACLsPreload(dbmodels.ACLsByIdentifiers(db, c.Args())).Find(&acls).Error; err != nil {
							return err
						}

						enc := json.NewEncoder(s)
						enc.SetIndent("", "  ")
						return enc.Encode(acls)
					},
				}, {
					Name:  "ls",
					Usage: "Lists ACLs",
					Flags: []cli.Flag{
						cli.BoolFlag{Name: "latest, l", Usage: "Show the latest ACL"},
						cli.BoolFlag{Name: "quiet, q", Usage: "Only display IDs"},
					},
					Action: func(c *cli.Context) error {
						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						var acls []*dbmodels.ACL
						query := db.Order("created_at desc").Preload("UserGroups").Preload("HostGroups")
						if c.Bool("latest") {
							var acl dbmodels.ACL
							if err := query.First(&acl).Error; err != nil {
								return err
							}
							acls = append(acls, &acl)
						} else if err := query.Find(&acls).Error; err != nil {
							return err
						}
						if c.Bool("quiet") {
							for _, acl := range acls {
								fmt.Fprintln(s, acl.ID)
							}
							return nil
						}

						table := tablewriter.NewWriter(s)
						table.SetHeader([]string{"ID", "Weight", "User groups", "Host groups", "Host pattern", "Action", "Inception", "Expiration", "Updated", "Created", "Comment"})
						table.SetBorder(false)
						table.SetCaption(true, fmt.Sprintf("Total: %d ACLs.", len(acls)))
						for _, acl := range acls {
							userGroups := []string{}
							hostGroups := []string{}
							for _, entity := range acl.UserGroups {
								userGroups = append(userGroups, entity.Name)
							}
							for _, entity := range acl.HostGroups {
								hostGroups = append(hostGroups, entity.Name)
							}

							inception := ""
							if acl.Inception != nil {
								inception = acl.Inception.Format("2006-01-02 15:04 MST")
							}
							expiration := ""
							if acl.Expiration != nil {
								expiration = acl.Expiration.Format("2006-01-02 15:04 MST")
							}

							table.Append([]string{
								fmt.Sprintf("%d", acl.ID),
								fmt.Sprintf("%d", acl.Weight),
								strings.Join(userGroups, ", "),
								strings.Join(hostGroups, ", "),
								acl.HostPattern,
								acl.Action,
								inception,
								expiration,
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
					Usage:     "Removes one or more ACLs",
					ArgsUsage: "ACL...",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}
						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						return dbmodels.ACLsByIdentifiers(db, c.Args()).Delete(&dbmodels.ACL{}).Error
					},
				}, {
					Name:      "update",
					Usage:     "Updates an existing acl",
					ArgsUsage: "ACL...",
					Flags: []cli.Flag{
						cli.StringFlag{Name: "action, a", Usage: "Update action"},
						cli.StringFlag{Name: "pattern, p", Usage: "Update host-pattern"},
						cli.UintFlag{Name: "weight, w", Usage: "Update weight"},
						cli.StringFlag{Name: "inception, i", Usage: "Update inception date-time"},
						cli.BoolFlag{Name: "unset-inception", Usage: "Unset inception date-time"},
						cli.BoolFlag{Name: "unset-expiration", Usage: "Unset expiration date-time"},
						cli.StringFlag{Name: "expiration, e", Usage: "Update expiration date-time"},
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
						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						var acls []*dbmodels.ACL
						if err := dbmodels.ACLsByIdentifiers(db, c.Args()).Find(&acls).Error; err != nil {
							return err
						}

						tx := db.Begin()
						for _, acl := range acls {
							model := tx.Model(acl)
							inception, err := parseOptionalTime(c.String("inception"))
							if err != nil {
								return err
							}
							expiration, err := parseOptionalTime(c.String("expiration"))
							if err != nil {
								return err
							}

							update := dbmodels.ACL{
								Action:      c.String("action"),
								HostPattern: c.String("pattern"),
								Weight:      c.Uint("weight"),
								Inception:   inception,
								Expiration:  expiration,
								Comment:     c.String("comment"),
							}
							if err := model.Updates(update).Error; err != nil {
								tx.Rollback()
								return err
							}

							if c.Bool("unset-inception") {
								if err := model.Update("inception", nil).Error; err != nil {
									tx.Rollback()
									return err
								}
							}
							if c.Bool("unset-expiration") {
								if err := model.Update("expiration", nil).Error; err != nil {
									tx.Rollback()
									return err
								}
							}

							// associations
							var appendUserGroups []dbmodels.UserGroup
							var deleteUserGroups []dbmodels.UserGroup
							if err := dbmodels.UserGroupsByIdentifiers(db, c.StringSlice("assign-usergroup")).Find(&appendUserGroups).Error; err != nil {
								tx.Rollback()
								return err
							}
							if err := dbmodels.UserGroupsByIdentifiers(db, c.StringSlice("unassign-usergroup")).Find(&deleteUserGroups).Error; err != nil {
								tx.Rollback()
								return err
							}
							if err := model.Association("UserGroups").Append(&appendUserGroups).Error; err != nil {
								tx.Rollback()
								return err
							}
							if len(deleteUserGroups) > 0 {
								if err := model.Association("UserGroups").Delete(deleteUserGroups).Error; err != nil {
									tx.Rollback()
									return err
								}
							}

							var appendHostGroups []dbmodels.HostGroup
							var deleteHostGroups []dbmodels.HostGroup
							if err := dbmodels.HostGroupsByIdentifiers(db, c.StringSlice("assign-hostgroup")).Find(&appendHostGroups).Error; err != nil {
								tx.Rollback()
								return err
							}
							if err := dbmodels.HostGroupsByIdentifiers(db, c.StringSlice("unassign-hostgroup")).Find(&deleteHostGroups).Error; err != nil {
								tx.Rollback()
								return err
							}
							if err := model.Association("HostGroups").Append(&appendHostGroups).Error; err != nil {
								tx.Rollback()
								return err
							}
							if len(deleteHostGroups) > 0 {
								if err := model.Association("HostGroups").Delete(deleteHostGroups).Error; err != nil {
									tx.Rollback()
									return err
								}
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
						cli.BoolFlag{Name: "decrypt", Usage: "decrypt sensitive data"},
						cli.BoolFlag{Name: "ignore-events", Usage: "do not backup events data"},
					},
					Description: "ssh admin@portal config backup > sshportal.bkp",
					Action: func(c *cli.Context) error {
						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						config := dbmodels.Config{}
						if err := dbmodels.HostsPreload(db).Find(&config.Hosts).Error; err != nil {
							return err
						}

						if err := dbmodels.SSHKeysPreload(db).Find(&config.SSHKeys).Error; err != nil {
							return err
						}
						for _, key := range config.SSHKeys {
							crypto.SSHKeyDecrypt(actx.aesKey, key)
						}
						if !c.Bool("decrypt") {
							for _, key := range config.SSHKeys {
								if err := crypto.SSHKeyEncrypt(actx.aesKey, key); err != nil {
									return err
								}
							}
						}

						if err := dbmodels.HostsPreload(db).Find(&config.Hosts).Error; err != nil {
							return err
						}
						for _, host := range config.Hosts {
							crypto.HostDecrypt(actx.aesKey, host)
						}
						if !c.Bool("decrypt") {
							for _, host := range config.Hosts {
								if err := crypto.HostEncrypt(actx.aesKey, host); err != nil {
									return err
								}
							}
						}

						if err := dbmodels.UserKeysPreload(db).Find(&config.UserKeys).Error; err != nil {
							return err
						}
						if err := dbmodels.UsersPreload(db).Find(&config.Users).Error; err != nil {
							return err
						}
						if err := dbmodels.UserGroupsPreload(db).Find(&config.UserGroups).Error; err != nil {
							return err
						}
						if err := dbmodels.HostGroupsPreload(db).Find(&config.HostGroups).Error; err != nil {
							return err
						}
						if err := dbmodels.ACLsPreload(db).Find(&config.ACLs).Error; err != nil {
							return err
						}
						if err := db.Find(&config.Settings).Error; err != nil {
							return err
						}
						if err := dbmodels.SessionsPreload(db).Find(&config.Sessions).Error; err != nil {
							return err
						}
						if !c.Bool("ignore-events") {
							if err := dbmodels.EventsPreload(db).Find(&config.Events).Error; err != nil {
								return err
							}
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
						cli.BoolFlag{Name: "decrypt", Usage: "do not encrypt sensitive data"},
					},
					Action: func(c *cli.Context) error {
						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						config := dbmodels.Config{}

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
						fmt.Fprintf(s, "* %d Sessions\n", len(config.Sessions))
						fmt.Fprintf(s, "* %d Events\n", len(config.Events))

						if !c.Bool("confirm") {
							fmt.Fprintf(s, "restore will erase and replace everything in the database.\nIf you are ok, add the '--confirm' to the restore command\n")
							return errors.New("")
						}

						tx := db.Begin()

						// FIXME: handle different migrations:
						//   1. drop tables
						//   2. apply migrations `1` to `<backup-migration-id>`
						//   3. restore data
						//   4. continues migrations

						// FIXME: tell the administrator to restart the server
						// if the master host key changed

						// FIXME: do everything in a transaction
						tableNames := []string{
							"acls",
							"events",
							"host_group_acls",
							"host_groups",
							"host_host_groups",
							"hosts",
							//"migrations",
							"sessions",
							"settings",
							"ssh_keys",
							"user_group_acls",
							"user_groups",
							"user_keys",
							"user_roles",
							"user_user_groups",
							"user_user_roles",
							"users",
						}
						for _, tableName := range tableNames {
							/* #nosec */
							if err := tx.Exec(fmt.Sprintf("DELETE FROM %s", tableName)).Error; err != nil {
								tx.Rollback()
								return err
							}
						}
						for _, host := range config.Hosts {
							host := host
							crypto.HostDecrypt(actx.aesKey, host)
							if !c.Bool("decrypt") {
								if err := crypto.HostEncrypt(actx.aesKey, host); err != nil {
									return err
								}
							}
							if err := tx.FirstOrCreate(&host).Error; err != nil {
								tx.Rollback()
								return err
							}
						}
						for _, user := range config.Users {
							user := user
							if err := tx.FirstOrCreate(&user).Error; err != nil {
								tx.Rollback()
								return err
							}
						}
						for _, acl := range config.ACLs {
							acl := acl
							if err := tx.FirstOrCreate(&acl).Error; err != nil {
								tx.Rollback()
								return err
							}
						}
						for _, hostGroup := range config.HostGroups {
							hostGroup := hostGroup
							if err := tx.FirstOrCreate(&hostGroup).Error; err != nil {
								tx.Rollback()
								return err
							}
						}
						for _, userGroup := range config.UserGroups {
							userGroup := userGroup
							if err := tx.FirstOrCreate(&userGroup).Error; err != nil {
								tx.Rollback()
								return err
							}
						}
						for _, sshKey := range config.SSHKeys {
							sshKey := sshKey
							crypto.SSHKeyDecrypt(actx.aesKey, sshKey)
							if !c.Bool("decrypt") {
								if err := crypto.SSHKeyEncrypt(actx.aesKey, sshKey); err != nil {
									return err
								}
							}
							if err := tx.FirstOrCreate(&sshKey).Error; err != nil {
								tx.Rollback()
								return err
							}
						}
						for _, userKey := range config.UserKeys {
							userKey := userKey
							if err := tx.FirstOrCreate(&userKey).Error; err != nil {
								tx.Rollback()
								return err
							}
						}
						for _, setting := range config.Settings {
							setting := setting
							if err := tx.FirstOrCreate(&setting).Error; err != nil {
								tx.Rollback()
								return err
							}
						}
						for _, session := range config.Sessions {
							session := session
							if err := tx.FirstOrCreate(&session).Error; err != nil {
								tx.Rollback()
								return err
							}
						}
						for _, event := range config.Events {
							event := event
							if err := tx.FirstOrCreate(&event).Error; err != nil {
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
			Name:  "event",
			Usage: "Manages events",
			Subcommands: []cli.Command{
				{
					Name:      "inspect",
					Usage:     "Shows detailed information on one or more events",
					ArgsUsage: "EVENT...",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						var events []*dbmodels.Event
						if err := dbmodels.EventsPreload(dbmodels.EventsByIdentifiers(db, c.Args())).Find(&events).Error; err != nil {
							return err
						}

						for _, event := range events {
							if len(event.Args) > 0 {
								if err := json.Unmarshal(event.Args, &event.ArgsMap); err != nil {
									return err
								}
							}
						}

						enc := json.NewEncoder(s)
						enc.SetIndent("", "  ")
						return enc.Encode(events)
					},
				}, {
					Name:  "ls",
					Usage: "Lists events",
					Flags: []cli.Flag{
						cli.BoolFlag{Name: "latest, l", Usage: "Show the latest event"},
						cli.BoolFlag{Name: "quiet, q", Usage: "Only display IDs"},
					},
					Action: func(c *cli.Context) error {
						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						var events []dbmodels.Event
						query := db.Order("created_at desc").Preload("Author")
						if c.Bool("latest") {
							var event dbmodels.Event
							if err := query.First(&event).Error; err != nil {
								return err
							}
							events = append(events, event)
						} else if err := query.Find(&events).Error; err != nil {
							return err
						}

						if c.Bool("quiet") {
							for _, event := range events {
								fmt.Fprintln(s, event.ID)
							}
							return nil
						}

						table := tablewriter.NewWriter(s)
						table.SetHeader([]string{"ID", "Author", "Domain", "Action", "Entity", "Args", "Date"})
						table.SetBorder(false)
						table.SetCaption(true, fmt.Sprintf("Total: %d events.", len(events)))
						for _, event := range events {
							author := ""
							if event.Author != nil {
								author = event.Author.Name
							}
							table.Append([]string{
								fmt.Sprintf("%d", event.ID),
								author,
								event.Domain,
								event.Action,
								event.Entity,
								wrapText(string(event.Args), 30),
								humanize.Time(event.CreatedAt),
							})
						}
						table.Render()
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
					ArgsUsage:   "[scheme://]<user>[:<password>]@<host>[:<port>]",
					Description: "$> host create bart@foo.org\n   $> host create bob:marley@example.com:2222",
					Flags: []cli.Flag{
						cli.StringFlag{Name: "name, n", Usage: "Assigns a name to the host"},
						cli.StringFlag{Name: "password, p", Usage: "If present, sshportal will use password-based authentication"},
						cli.StringFlag{Name: "comment, c"},
						cli.StringFlag{Name: "key, k", Usage: "`KEY` to use for authentication"},
						cli.StringFlag{Name: "hop, o", Usage: "Hop to use for connecting to the server"},
						cli.StringFlag{Name: "logging, l", Usage: "Logging mode (disabled, input, everything)"},
						cli.StringSliceFlag{Name: "group, g", Usage: "Assigns the host to `HOSTGROUPS` (default: \"default\")"},
					},
					Action: func(c *cli.Context) error {
						if c.NArg() != 1 {
							return cli.ShowSubcommandHelp(c)
						}

						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						u, err := parseInputURL(c.Args().First())
						if err != nil {
							return err
						}
						host := &dbmodels.Host{
							URL:     u.String(),
							Comment: c.String("comment"),
						}
						if c.String("password") != "" {
							host.Password = c.String("password")
						}
						matched, err := regexp.MatchString(`^([0-9]{1,3}.){3}.([0-9]{1,3})$`, host.Hostname())
						if err != nil {
							return err
						}
						if matched {
							host.Name = host.Hostname()
						} else {
							host.Name = strings.Split(host.Hostname(), ".")[0]
						}

						if c.String("hop") != "" {
							hop, err := dbmodels.HostByName(db, c.String("hop"))
							if err != nil {
								return err
							}
							host.Hop = hop
						}
						if c.String("name") != "" {
							host.Name = c.String("name")
						}

						host.Logging = "everything" // default is everything
						if c.String("logging") != "" {
							host.Logging = c.String("logging")
						}
						// FIXME: check if name already exists

						if _, err := govalidator.ValidateStruct(host); err != nil {
							return err
						}

						inputKey := c.String("key")
						if inputKey == "" && host.Password == "" {
							inputKey = "default"
						}
						if inputKey != "" {
							var key dbmodels.SSHKey
							if err := dbmodels.SSHKeysByIdentifiers(db, []string{inputKey}).First(&key).Error; err != nil {
								return err
							}
							host.SSHKeyID = key.ID
						}

						// host group
						inputGroups := c.StringSlice("group")
						if len(inputGroups) == 0 {
							inputGroups = []string{"default"}
						}
						if err := dbmodels.HostGroupsByIdentifiers(db, inputGroups).Find(&host.Groups).Error; err != nil {
							return err
						}

						// encrypt
						if err := crypto.HostEncrypt(actx.aesKey, host); err != nil {
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
					Flags: []cli.Flag{
						cli.BoolFlag{Name: "decrypt", Usage: "Decrypt sensitive data"},
					},
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						if err := myself.CheckRoles([]string{"admin", "listhosts"}); err != nil {
							return err
						}

						var hosts []*dbmodels.Host
						db = db.Preload("Groups")
						if myself.HasRole("admin") {
							db = db.Preload("SSHKey")
						}
						if err := dbmodels.HostsByIdentifiers(db, c.Args()).Find(&hosts).Error; err != nil {
							return err
						}

						if c.Bool("decrypt") {
							for _, host := range hosts {
								crypto.HostDecrypt(actx.aesKey, host)
							}
						}

						enc := json.NewEncoder(s)
						enc.SetIndent("", "  ")
						return enc.Encode(hosts)
					},
				}, {
					Name:  "ls",
					Usage: "Lists hosts",
					Flags: []cli.Flag{
						cli.BoolFlag{Name: "latest, l", Usage: "Show the latest host"},
						cli.BoolFlag{Name: "quiet, q", Usage: "Only display IDs"},
					},
					Action: func(c *cli.Context) error {
						if err := myself.CheckRoles([]string{"admin", "listhosts"}); err != nil {
							return err
						}

						var hosts []*dbmodels.Host
						query := db.Order("created_at desc").Preload("Groups")
						if c.Bool("latest") {
							var host dbmodels.Host
							if err := query.First(&host).Error; err != nil {
								return err
							}
							hosts = append(hosts, &host)
						} else if err := query.Find(&hosts).Error; err != nil {
							return err
						}

						if c.Bool("quiet") {
							for _, host := range hosts {
								fmt.Fprintln(s, host.ID)
							}
							return nil
						}

						table := tablewriter.NewWriter(s)
						table.SetHeader([]string{"ID", "Name", "URL", "Key", "Groups", "Updated", "Created", "Comment", "Hop", "Logging"})
						table.SetBorder(false)
						table.SetCaption(true, fmt.Sprintf("Total: %d hosts.", len(hosts)))
						for _, host := range hosts {
							authKey := ""
							if host.SSHKeyID > 0 {
								var key dbmodels.SSHKey
								db.Model(host).Related(&key)
								authKey = key.Name
							}
							groupNames := []string{}
							for _, hostGroup := range host.Groups {
								groupNames = append(groupNames, hostGroup.Name)
							}
							var hop string
							if host.HopID != 0 {
								var hopHost dbmodels.Host
								db.Model(host).Related(&hopHost, "HopID")
								hop = hopHost.Name
							} else {
								hop = ""
							}
							table.Append([]string{
								fmt.Sprintf("%d", host.ID),
								host.Name,
								host.String(),
								authKey,
								strings.Join(groupNames, ", "),
								humanize.Time(host.UpdatedAt),
								humanize.Time(host.CreatedAt),
								host.Comment,
								hop,
								host.Logging,
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

						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						return dbmodels.HostsByIdentifiers(db, c.Args()).Delete(&dbmodels.Host{}).Error
					},
				}, {
					Name:      "update",
					Usage:     "Updates an existing host",
					ArgsUsage: "HOST...",
					Flags: []cli.Flag{
						cli.StringFlag{Name: "name, n", Usage: "Rename the host"},
						cli.StringFlag{Name: "url, u", Usage: "Update connection URL"},
						cli.StringFlag{Name: "comment, c", Usage: "Update/set a host comment"},
						cli.StringFlag{Name: "key, k", Usage: "Link a `KEY` to use for authentication"},
						cli.StringFlag{Name: "hop, o", Usage: "Change the hop to use for connecting to the server"},
						cli.StringFlag{Name: "logging, l", Usage: "Logging mode (disabled, input, everything)"},
						cli.BoolFlag{Name: "unset-hop", Usage: "Remove the hop set for this host"},
						cli.StringSliceFlag{Name: "assign-group, g", Usage: "Assign the host to a new `HOSTGROUPS`"},
						cli.StringSliceFlag{Name: "unassign-group", Usage: "Unassign the host from a `HOSTGROUPS`"},
					},
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						var hosts []dbmodels.Host
						if err := dbmodels.HostsByIdentifiers(db, c.Args()).Find(&hosts).Error; err != nil {
							return err
						}

						if len(hosts) > 1 && c.String("name") != "" {
							return fmt.Errorf("cannot set --name when editing multiple hosts at once")
						}

						tx := db.Begin()
						for _, host := range hosts {
							host := host
							model := tx.Model(&host)
							// simple fields
							for _, fieldname := range []string{"name", "comment"} {
								if c.String(fieldname) != "" {
									if err := model.Update(fieldname, c.String(fieldname)).Error; err != nil {
										tx.Rollback()
										return err
									}
								}
							}

							// url
							if c.String("url") != "" {
								u, err := parseInputURL(c.String("url"))
								if err != nil {
									tx.Rollback()
									return err
								}
								if err := model.Update("url", u.String()).Error; err != nil {
									tx.Rollback()
									return err
								}
							}

							// hop
							if c.String("hop") != "" {
								hop, err := dbmodels.HostByName(db, c.String("hop"))
								if err != nil {
									tx.Rollback()
									return err
								}
								if err := model.Association("Hop").Replace(hop).Error; err != nil {
									tx.Rollback()
									return err
								}
							}

							// logging
							if logging := c.String("logging"); logging != "" {
								if !dbmodels.IsValidHostLoggingMode(logging) {
									return fmt.Errorf("invalid host logging mode: %q", logging)
								}
								if err := model.Update("logging", logging).Error; err != nil {
									tx.Rollback()
									return err
								}
							}

							// remove the hop
							if c.Bool("unset-hop") {
								var hopHost dbmodels.Host

								db.Model(&host).Related(&hopHost, "HopID")
								if err := model.Association("Hop").Clear().Error; err != nil {
									tx.Rollback()
									return err
								}
							}

							// associations
							if c.String("key") != "" {
								var key dbmodels.SSHKey
								if err := dbmodels.SSHKeysByIdentifiers(db, []string{c.String("key")}).First(&key).Error; err != nil {
									tx.Rollback()
									return err
								}
								if err := model.Association("SSHKey").Replace(&key).Error; err != nil {
									tx.Rollback()
									return err
								}
							}
							var appendGroups []dbmodels.HostGroup
							var deleteGroups []dbmodels.HostGroup
							if err := dbmodels.HostGroupsByIdentifiers(db, c.StringSlice("assign-group")).Find(&appendGroups).Error; err != nil {
								tx.Rollback()
								return err
							}
							if err := dbmodels.HostGroupsByIdentifiers(db, c.StringSlice("unassign-group")).Find(&deleteGroups).Error; err != nil {
								tx.Rollback()
								return err
							}
							if err := model.Association("Groups").Append(&appendGroups).Error; err != nil {
								tx.Rollback()
								return err
							}
							if len(deleteGroups) > 0 {
								if err := model.Association("Groups").Delete(deleteGroups).Error; err != nil {
									tx.Rollback()
									return err
								}
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
						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						hostGroup := dbmodels.HostGroup{
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

						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						var hostGroups []dbmodels.HostGroup
						if err := dbmodels.HostGroupsPreload(dbmodels.HostGroupsByIdentifiers(db, c.Args())).Find(&hostGroups).Error; err != nil {
							return err
						}

						enc := json.NewEncoder(s)
						enc.SetIndent("", "  ")
						return enc.Encode(hostGroups)
					},
				}, {
					Name:  "ls",
					Usage: "Lists host groups",
					Flags: []cli.Flag{
						cli.BoolFlag{Name: "latest, l", Usage: "Show the latest host group"},
						cli.BoolFlag{Name: "quiet, q", Usage: "Only display IDs"},
					},
					Action: func(c *cli.Context) error {
						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						var hostGroups []*dbmodels.HostGroup
						query := db.Order("created_at desc").Preload("ACLs").Preload("Hosts")
						if c.Bool("latest") {
							var hostGroup dbmodels.HostGroup
							if err := query.First(&hostGroup).Error; err != nil {
								return err
							}
							hostGroups = append(hostGroups, &hostGroup)
						} else if err := query.Find(&hostGroups).Error; err != nil {
							return err
						}

						if c.Bool("quiet") {
							for _, hostGroup := range hostGroups {
								fmt.Fprintln(s, hostGroup.ID)
							}
							return nil
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

						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						return dbmodels.HostGroupsByIdentifiers(db, c.Args()).Delete(&dbmodels.HostGroup{}).Error
					},
				}, {
					Name:      "update",
					Usage:     "Updates a host group",
					ArgsUsage: "HOSTGROUP...",
					Flags: []cli.Flag{
						cli.StringFlag{Name: "name", Usage: "Assigns a new name to the host group"},
						cli.StringFlag{Name: "comment", Usage: "Adds a comment"},
					},
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						var hostgroups []*dbmodels.HostGroup
						if err := dbmodels.HostGroupsByIdentifiers(db, c.Args()).Find(&hostgroups).Error; err != nil {
							return err
						}

						if len(hostgroups) > 1 && c.String("name") != "" {
							return fmt.Errorf("cannot set --name when editing multiple hostgroups at once")
						}

						tx := db.Begin()
						for _, hostgroup := range hostgroups {
							model := tx.Model(hostgroup)
							// simple fields
							for _, fieldname := range []string{"name", "comment"} {
								if c.String(fieldname) != "" {
									if err := model.Update(fieldname, c.String(fieldname)).Error; err != nil {
										tx.Rollback()
										return err
									}
								}
							}
						}
						return tx.Commit().Error
					},
				},
			},
		}, {
			Name:  "info",
			Usage: "Shows system-wide information",
			Action: func(c *cli.Context) error {
				if err := myself.CheckRoles([]string{"admin"}); err != nil {
					return err
				}

				fmt.Fprintf(s, "debug mode (server): %v\n", actx.debug)
				hostname, _ := os.Hostname()
				fmt.Fprintf(s, "Hostname: %s\n", hostname)
				fmt.Fprintf(s, "CPUs: %d\n", runtime.NumCPU())
				fmt.Fprintf(s, "Demo mode: %v\n", actx.demo)
				fmt.Fprintf(s, "DB Driver: %s\n", actx.dbDriver)
				fmt.Fprintf(s, "DB Conn: %s\n", actx.dbURL)
				fmt.Fprintf(s, "Bind Address: %s\n", actx.bindAddr)
				fmt.Fprintf(s, "System Time: %v\n", time.Now().Format(time.RFC3339Nano))
				fmt.Fprintf(s, "OS Type: %s\n", runtime.GOOS)
				fmt.Fprintf(s, "OS Architecture: %s\n", runtime.GOARCH)
				fmt.Fprintf(s, "Go routines: %d\n", runtime.NumGoroutine())
				fmt.Fprintf(s, "Go version (build): %v\n", runtime.Version())
				fmt.Fprintf(s, "Uptime: %v\n", time.Since(startTime))

				fmt.Fprintf(s, "User ID: %v\n", myself.ID)
				fmt.Fprintf(s, "User email: %s\n", myself.Email)
				fmt.Fprintf(s, "Version: %s\n", version)
				fmt.Fprintf(s, "GIT SHA: %s\n", gitSha)
				fmt.Fprintf(s, "GIT Tag: %s\n", gitTag)

				// FIXME: gormigrate version
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
						cli.StringFlag{Name: "type", Value: "ed25519"},
						cli.UintFlag{Name: "length", Value: 0},
						cli.StringFlag{Name: "comment", Usage: "Adds a comment"},
					},
					Action: func(c *cli.Context) error {
						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						name := namesgenerator.GetRandomName(0)
						if c.String("name") != "" {
							name = c.String("name")
						}

						length := c.Uint("length")
						if length == 0 {
							switch c.String("type") {
							case "rsa":
								// same default as ssh-keygen
								length = 3072
							case "ecdsa":
								// same default as ssh-keygen
								length = 256
							case "ed25519":
								// irrelevant for ed25519
								// set it to 1 to enforce consistency
								// and because 0 is invalid
								length = 1
							}
						}

						key, err := crypto.NewSSHKey(c.String("type"), length)
						if actx.aesKey != "" {
							if err2 := crypto.SSHKeyEncrypt(actx.aesKey, key); err2 != nil {
								return err2
							}
						}
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
					Name:        "import",
					Usage:       "Imports an existing private key",
					Description: "$> key import\n   $> key import --name=mykey",
					Flags: []cli.Flag{
						cli.StringFlag{Name: "name", Usage: "Assigns a name to the key"},
						cli.StringFlag{Name: "comment", Usage: "Adds a comment"},
					},
					Action: func(c *cli.Context) error {
						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						var name string
						if c.String("name") != "" {
							name = c.String("name")
						} else {
							name = namesgenerator.GetRandomName(0)
						}

						var value string
						term := terminal.NewTerminal(s, "Paste your key and end with a blank line> ")
						for {
							line, err := term.ReadLine()
							if err != nil {
								return err
							}
							if line != "" {
								value += line + "\n"
							} else {
								break
							}
						}
						key, err := crypto.ImportSSHKey(value)
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
					Flags: []cli.Flag{
						cli.BoolFlag{Name: "decrypt", Usage: "Decrypt sensitive data"},
					},
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						var keys []*dbmodels.SSHKey
						if err := dbmodels.SSHKeysByIdentifiers(dbmodels.SSHKeysPreload(db), c.Args()).Find(&keys).Error; err != nil {
							return err
						}

						if c.Bool("decrypt") {
							for _, key := range keys {
								crypto.SSHKeyDecrypt(actx.aesKey, key)
							}
						}

						enc := json.NewEncoder(s)
						enc.SetIndent("", "  ")
						return enc.Encode(keys)
					},
				}, {
					Name:  "ls",
					Usage: "Lists keys",
					Flags: []cli.Flag{
						cli.BoolFlag{Name: "latest, l", Usage: "Show the latest key"},
						cli.BoolFlag{Name: "quiet, q", Usage: "Only display IDs"},
					},
					Action: func(c *cli.Context) error {
						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						var sshKeys []*dbmodels.SSHKey
						query := db.Order("created_at desc").Preload("Hosts")
						if c.Bool("latest") {
							var sshKey dbmodels.SSHKey
							if err := query.First(&sshKey).Error; err != nil {
								return err
							}
							sshKeys = append(sshKeys, &sshKey)
						} else if err := query.Find(&sshKeys).Error; err != nil {
							return err
						}
						if c.Bool("quiet") {
							for _, sshKey := range sshKeys {
								fmt.Fprintln(s, sshKey.ID)
							}
							return nil
						}

						table := tablewriter.NewWriter(s)
						table.SetHeader([]string{"ID", "Name", "Type", "Length", "Hosts", "Updated", "Created", "Comment"})
						table.SetBorder(false)
						table.SetCaption(true, fmt.Sprintf("Total: %d keys.", len(sshKeys)))
						for _, key := range sshKeys {
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

						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						return dbmodels.SSHKeysByIdentifiers(db, c.Args()).Delete(&dbmodels.SSHKey{}).Error
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

						var key dbmodels.SSHKey
						if err := dbmodels.SSHKeysByIdentifiers(db, c.Args()).First(&key).Error; err != nil {
							return err
						}
						fmt.Fprintf(s, "umask 077; mkdir -p .ssh; echo %s sshportal >> .ssh/authorized_keys\n", key.PubKey)
						return nil
					},
				}, {
					Name:      "show",
					Usage:     "Shows standard information on a `KEY`",
					ArgsUsage: "KEY",
					Action: func(c *cli.Context) error {
						if c.NArg() != 1 {
							return cli.ShowSubcommandHelp(c)
						}

						// not checking roles, everyone with an account can see how to enroll new hosts

						var key dbmodels.SSHKey
						if err := dbmodels.SSHKeysByIdentifiers(dbmodels.SSHKeysPreload(db), c.Args()).First(&key).Error; err != nil {
							return err
						}
						crypto.SSHKeyDecrypt(actx.aesKey, &key)

						type line struct {
							key   string
							value string
						}
						type section struct {
							name  string
							lines []line
						}
						var hosts []string
						for _, host := range key.Hosts {
							hosts = append(hosts, host.Name)
						}
						sections := []section{
							{
								name: "General",
								lines: []line{
									{"Name", key.Name},
									{"Type", key.Type},
									{"Length", fmt.Sprintf("%d", key.Length)},
									{"Comment", key.Comment},
								},
							}, {
								name: "Relationships",
								lines: []line{
									{"Linked hosts", fmt.Sprintf("%s (%d)", strings.Join(hosts, ", "), len(hosts))},
								},
							}, {
								name: "Crypto",
								lines: []line{
									{"authorized_key format", key.PubKey},
									{"Private Key", key.PrivKey},
								},
							}, {
								name: "Help",
								lines: []line{
									{"inspect", fmt.Sprintf("ssh sshportal key inspect %s", key.Name)},
									{"setup", fmt.Sprintf(`ssh user@example.com "$(ssh sshportal key setup %s)"`, key.Name)},
								},
							},
						}

						valueColor := ansi.ColorFunc("white")
						titleColor := ansi.ColorFunc("magenta+bh")
						keyColor := ansi.ColorFunc("red+bh")
						for _, section := range sections {
							fmt.Fprintf(s, "%s\n%s\n", titleColor(section.name), strings.Repeat("=", len(section.name)))
							for _, line := range section.lines {
								if strings.Contains(line.value, "\n") {
									fmt.Fprintf(s, "%s:\n%s\n", keyColor(line.key), valueColor(line.value))
								} else {
									fmt.Fprintf(s, "%s: %s\n", keyColor(line.key), valueColor(line.value))
								}
							}
							fmt.Fprintf(s, "\n")
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
					ArgsUsage: "USER...",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						var users []dbmodels.User
						if err := dbmodels.UsersPreload(dbmodels.UsersByIdentifiers(db, c.Args())).Find(&users).Error; err != nil {
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

						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						// FIXME: validate email

						email := c.Args().First()
						name := strings.Split(email, "@")[0]
						if c.String("name") != "" {
							name = c.String("name")
						}

						user := dbmodels.User{
							Name:        name,
							Email:       email,
							Comment:     c.String("comment"),
							InviteToken: randStringBytes(16),
						}

						if _, err := govalidator.ValidateStruct(user); err != nil {
							return err
						}

						// user group
						inputGroups := c.StringSlice("group")
						if len(inputGroups) == 0 {
							inputGroups = []string{"default"}
						}
						if err := dbmodels.UserGroupsByIdentifiers(db, inputGroups).Find(&user.Groups).Error; err != nil {
							return err
						}

						// save the user in database
						if err := db.Create(&user).Error; err != nil {
							return err
						}
						fmt.Fprintf(s, "User %d created.\nTo associate this account with a key, use the following SSH user: 'invite:%s'.\n", user.ID, user.InviteToken)
						return nil
					},
				}, {
					Name:  "ls",
					Usage: "Lists users",
					Flags: []cli.Flag{
						cli.BoolFlag{Name: "latest, l", Usage: "Show the latest user"},
						cli.BoolFlag{Name: "quiet, q", Usage: "Only display IDs"},
					},
					Action: func(c *cli.Context) error {
						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						var users []*dbmodels.User
						query := db.Order("created_at desc").Preload("Groups").Preload("Roles").Preload("Keys")
						if c.Bool("latest") {
							var user dbmodels.User
							if err := query.First(&user).Error; err != nil {
								return err
							}
							users = append(users, &user)
						} else if err := query.Find(&users).Error; err != nil {
							return err
						}
						if c.Bool("quiet") {
							for _, user := range users {
								fmt.Fprintln(s, user.ID)
							}
							return nil
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

						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						return dbmodels.UsersByIdentifiers(db, c.Args()).Delete(&dbmodels.User{}).Error
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

						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						// FIXME: check if unset-admin + user == myself
						var users []*dbmodels.User
						if err := dbmodels.UsersByIdentifiers(db, c.Args()).Find(&users).Error; err != nil {
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
							model := tx.Model(user)
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
							var appendGroups []dbmodels.UserGroup
							if err := dbmodels.UserGroupsByIdentifiers(db, c.StringSlice("assign-group")).Find(&appendGroups).Error; err != nil {
								tx.Rollback()
								return err
							}
							var deleteGroups []dbmodels.UserGroup
							if err := dbmodels.UserGroupsByIdentifiers(db, c.StringSlice("unassign-group")).Find(&deleteGroups).Error; err != nil {
								tx.Rollback()
								return err
							}
							if err := model.Association("Groups").Append(&appendGroups).Error; err != nil {
								tx.Rollback()
								return err
							}
							if len(deleteGroups) > 0 {
								if err := model.Association("Groups").Delete(deleteGroups).Error; err != nil {
									tx.Rollback()
									return err
								}
							}
							var appendRoles []dbmodels.UserRole
							if err := dbmodels.UserRolesByIdentifiers(db, c.StringSlice("assign-role")).Find(&appendRoles).Error; err != nil {
								tx.Rollback()
								return err
							}
							var deleteRoles []dbmodels.UserRole
							if err := dbmodels.UserRolesByIdentifiers(db, c.StringSlice("unassign-role")).Find(&deleteRoles).Error; err != nil {
								tx.Rollback()
								return err
							}
							if err := model.Association("Roles").Append(&appendRoles).Error; err != nil {
								tx.Rollback()
								return err
							}
							if len(deleteRoles) > 0 {
								if err := model.Association("Roles").Delete(deleteRoles).Error; err != nil {
									tx.Rollback()
									return err
								}
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
						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						userGroup := dbmodels.UserGroup{
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

						userGroup.Users = []*dbmodels.User{myself}

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

						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						var userGroups []dbmodels.UserGroup
						if err := dbmodels.UserGroupsPreload(dbmodels.UserGroupsByIdentifiers(db, c.Args())).Find(&userGroups).Error; err != nil {
							return err
						}

						enc := json.NewEncoder(s)
						enc.SetIndent("", "  ")
						return enc.Encode(userGroups)
					},
				}, {
					Name:  "ls",
					Usage: "Lists user groups",
					Flags: []cli.Flag{
						cli.BoolFlag{Name: "latest, l", Usage: "Show the latest user group"},
						cli.BoolFlag{Name: "quiet, q", Usage: "Only display IDs"},
					},
					Action: func(c *cli.Context) error {
						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						var userGroups []*dbmodels.UserGroup
						query := db.Order("created_at desc").Preload("ACLs").Preload("Users")
						if c.Bool("latest") {
							var userGroup dbmodels.UserGroup
							if err := query.First(&userGroup).Error; err != nil {
								return err
							}
							userGroups = append(userGroups, &userGroup)
						} else if err := query.Find(&userGroups).Error; err != nil {
							return err
						}
						if c.Bool("quiet") {
							for _, userGroup := range userGroups {
								fmt.Fprintln(s, userGroup.ID)
							}
							return nil
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

						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						return dbmodels.UserGroupsByIdentifiers(db, c.Args()).Delete(&dbmodels.UserGroup{}).Error
					},
				}, {
					Name:      "update",
					Usage:     "Updates a user group",
					ArgsUsage: "USERGROUP...",
					Flags: []cli.Flag{
						cli.StringFlag{Name: "name", Usage: "Assigns a new name to the user group"},
						cli.StringFlag{Name: "comment", Usage: "Adds a comment"},
					},
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						var usergroups []*dbmodels.UserGroup
						if err := dbmodels.UserGroupsByIdentifiers(db, c.Args()).Find(&usergroups).Error; err != nil {
							return err
						}

						if len(usergroups) > 1 && c.String("name") != "" {
							return fmt.Errorf("cannot set --name when editing multiple usergroups at once")
						}

						tx := db.Begin()
						for _, usergroup := range usergroups {
							model := tx.Model(usergroup)
							// simple fields
							for _, fieldname := range []string{"name", "comment"} {
								if c.String(fieldname) != "" {
									if err := model.Update(fieldname, c.String(fieldname)).Error; err != nil {
										tx.Rollback()
										return err
									}
								}
							}
						}
						return tx.Commit().Error
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

						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						var user dbmodels.User
						if err := dbmodels.UsersByIdentifiers(db, c.Args()).First(&user).Error; err != nil {
							return err
						}

						fmt.Fprintf(s, "Enter key:\n")
						reader := bufio.NewReader(s)
						text, _ := reader.ReadString('\n')

						key, comment, _, _, err := ssh.ParseAuthorizedKey([]byte(text))
						if err != nil {
							return err
						}

						userkey := dbmodels.UserKey{
							User:          &user,
							Key:           key.Marshal(),
							Comment:       comment,
							AuthorizedKey: string(gossh.MarshalAuthorizedKey(key)),
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

						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						var userKeys []dbmodels.UserKey
						if err := dbmodels.UserKeysPreload(dbmodels.UserKeysByIdentifiers(db, c.Args())).Find(&userKeys).Error; err != nil {
							return err
						}

						enc := json.NewEncoder(s)
						enc.SetIndent("", "  ")
						return enc.Encode(userKeys)
					},
				}, {
					Name:  "ls",
					Usage: "Lists userkeys",
					Flags: []cli.Flag{
						cli.BoolFlag{Name: "latest, l", Usage: "Show the latest user key"},
						cli.BoolFlag{Name: "quiet, q", Usage: "Only display IDs"},
					},
					Action: func(c *cli.Context) error {
						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						var userKeys []*dbmodels.UserKey
						query := db.Order("created_at desc").Preload("User")
						if c.Bool("latest") {
							var userKey dbmodels.UserKey
							if err := query.First(&userKey).Error; err != nil {
								return err
							}
							userKeys = append(userKeys, &userKey)
						} else if err := query.Find(&userKeys).Error; err != nil {
							return err
						}
						if c.Bool("quiet") {
							for _, userKey := range userKeys {
								fmt.Fprintln(s, userKey.ID)
							}
							return nil
						}

						table := tablewriter.NewWriter(s)
						table.SetHeader([]string{"ID", "User", "Updated", "Created", "Comment"})
						table.SetBorder(false)
						table.SetCaption(true, fmt.Sprintf("Total: %d userkeys.", len(userKeys)))
						for _, userkey := range userKeys {
							email := naMessage
							if userkey.User != nil {
								email = userkey.User.Email
							}
							table.Append([]string{
								fmt.Sprintf("%d", userkey.ID),
								email,
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

						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}
						if err := dbmodels.UserKeysByIdentifiers(db, c.Args()).Find(&dbmodels.UserKey{}).Error; err != nil {
							var user dbmodels.User
							if err := dbmodels.UsersByIdentifiers(db, c.Args()).First(&user).Error; err != nil {
								return err
							}
							if err := dbmodels.UserKeysByUserID(db, []string{fmt.Sprint(user.ID)}).Find(&dbmodels.UserKey{}).Error; err != nil {
								return err
							}
							return dbmodels.UserKeysByUserID(db, []string{fmt.Sprint(user.ID)}).Delete(&dbmodels.UserKey{}).Error
						}
						return dbmodels.UserKeysByIdentifiers(db, c.Args()).Delete(&dbmodels.UserKey{}).Error
					},
				},
			},
		}, {
			Name:  "session",
			Usage: "Manages sessions",
			Subcommands: []cli.Command{
				{
					Name:      "inspect",
					Usage:     "Shows detailed information on one or more sessions",
					ArgsUsage: "SESSION...",
					Action: func(c *cli.Context) error {
						if c.NArg() < 1 {
							return cli.ShowSubcommandHelp(c)
						}

						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						var sessions []dbmodels.Session
						if err := dbmodels.SessionsPreload(dbmodels.SessionsByIdentifiers(db, c.Args())).Find(&sessions).Error; err != nil {
							return err
						}

						enc := json.NewEncoder(s)
						enc.SetIndent("", "  ")
						return enc.Encode(sessions)
					},
				}, {
					Name:  "ls",
					Usage: "Lists sessions",
					Flags: []cli.Flag{
						cli.BoolFlag{Name: "latest, l", Usage: "Show the latest session"},
						cli.BoolFlag{Name: "active, a", Usage: "Show only active session"},
						cli.BoolFlag{Name: "quiet, q", Usage: "Only display IDs"},
					},
					Action: func(c *cli.Context) error {
						if err := myself.CheckRoles([]string{"admin"}); err != nil {
							return err
						}

						var sessions []*dbmodels.Session

						limit, offset, status := 60000, -1, []string{string(dbmodels.SessionStatusActive), string(dbmodels.SessionStatusClosed), string(dbmodels.SessionStatusUnknown)}
						if c.Bool("active") {
							status = status[:1]
						}

						query := db.Order("created_at desc").Limit(limit).Offset(offset).Where("status in (?)", status).Preload("User").Preload("Host")

						if c.Bool("latest") {
							var session dbmodels.Session
							if err := query.First(&session).Error; err != nil {
								return err
							}
							sessions = append(sessions, &session)
						} else {
							if err := query.Find(&sessions).Error; err != nil {
								return err
							}

							factor := 1
							for len(sessions) >= limit*factor {
								var additionnalSessions []*dbmodels.Session

								offset = limit * factor
								query := db.Order("created_at desc").Limit(limit).Offset(offset).Where("status in (?)", status).Preload("User").Preload("Host")
								if err := query.Find(&additionnalSessions).Error; err != nil {
									return err
								}
								sessions = append(sessions, additionnalSessions...)
								factor++
							}
						}
						if c.Bool("quiet") {
							for _, session := range sessions {
								fmt.Fprintln(s, session.ID)
							}
							return nil
						}

						table := tablewriter.NewWriter(s)
						table.SetHeader([]string{"ID", "User", "Host", "Status", "Start", "Duration", "Error", "Comment"})
						table.SetBorder(false)
						table.SetCaption(true, fmt.Sprintf("Total: %d sessions.", len(sessions)))
						for _, session := range sessions {
							var duration string
							if session.StoppedAt == nil || session.StoppedAt.IsZero() {
								duration = humanize.RelTime(session.CreatedAt, time.Now(), "", "")
							} else {
								duration = humanize.RelTime(session.CreatedAt, *session.StoppedAt, "", "")
							}
							duration = strings.Replace(duration, "now", "1 second", 1)
							hostname := naMessage
							if session.Host != nil {
								hostname = session.Host.Name
							}
							username := naMessage
							if session.User != nil {
								username = session.User.Name
							}
							table.Append([]string{
								fmt.Sprintf("%d", session.ID),
								username,
								hostname,
								session.Status,
								humanize.Time(session.CreatedAt),
								duration,
								wrapText(session.ErrMsg, 30),
								session.Comment,
							})
						}
						table.Render()
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
				fmt.Fprint(s, "syntax error.\n")
				continue
			}
			if len(words) == 1 && strings.ToLower(words[0]) == "exit" {
				return s.Exit(0)
			}
			if len(words) == 0 {
				continue
			}
			dbmodels.NewEvent("shell", words[0]).SetAuthor(myself).SetArg("interactive", true).SetArg("args", words[1:]).Log(db)
			if err := app.Run(append([]string{"config"}, words...)); err != nil {
				if cliErr, ok := err.(*cli.ExitError); ok {
					if cliErr.ExitCode() != 0 {
						fmt.Fprintf(s, "error: %v\n", err)
					}
					//s.Exit(cliErr.ExitCode())
				} else {
					fmt.Fprintf(s, "error: %v\n", err)
				}
			}
		}
	} else { // oneshot mode
		dbmodels.NewEvent("shell", sshCommand[0]).SetAuthor(myself).SetArg("interactive", false).SetArg("args", sshCommand[1:]).Log(db)
		if err := app.Run(append([]string{"config"}, sshCommand...)); err != nil {
			if errMsg := err.Error(); errMsg != "" {
				fmt.Fprintf(s, "error: %s\n", errMsg)
			}
			if cliErr, ok := err.(*cli.ExitError); ok {
				return s.Exit(cliErr.ExitCode())
			}
			return s.Exit(1)
		}
	}

	return nil
}

func wrapText(in string, length int) string {
	if len(in) <= length {
		return in
	}
	return in[0:length-3] + "..."
}

func parseInputURL(input string) (*url.URL, error) {
	if !strings.Contains(input, "://") {
		input = "ssh://" + input
	}
	u, err := url.Parse(input)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func parseOptionalTime(input string) (*time.Time, error) {
	if input != "" {
		parsed, err := time.ParseInLocation("2006-01-02 15:04", input, time.Local)
		if err != nil {
			return nil, err
		}
		return &parsed, nil
	}
	return nil, nil
}
