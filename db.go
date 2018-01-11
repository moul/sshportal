package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/jinzhu/gorm"
	gossh "golang.org/x/crypto/ssh"
)

type Config struct {
	SSHKeys    []*SSHKey    `json:"keys"`
	Hosts      []*Host      `json:"hosts"`
	UserKeys   []*UserKey   `json:"user_keys"`
	Users      []*User      `json:"users"`
	UserGroups []*UserGroup `json:"user_groups"`
	HostGroups []*HostGroup `json:"host_groups"`
	ACLs       []*ACL       `json:"acls"`
	Settings   []*Setting   `json:"settings"`
	Events     []*Event     `json:"events"`
	Sessions   []*Session   `json:"sessions"`
	// FIXME: add latest migration
	Date time.Time `json:"date"`
}

type Setting struct {
	gorm.Model
	Name  string `valid:"required"`
	Value string `valid:"required"`
}

// SSHKey defines a ssh client key (used by sshportal to connect to remote hosts)
type SSHKey struct {
	// FIXME: use uuid for ID
	gorm.Model
	Name        string  `valid:"required,length(1|32),unix_user"`
	Type        string  `valid:"required"`
	Length      uint    `valid:"required"`
	Fingerprint string  `valid:"optional"`
	PrivKey     string  `sql:"size:10000" valid:"required"`
	PubKey      string  `sql:"size:10000" valid:"optional"`
	Hosts       []*Host `gorm:"ForeignKey:SSHKeyID"`
	Comment     string  `valid:"optional"`
}

type Host struct {
	// FIXME: use uuid for ID
	gorm.Model
	Name     string       `gorm:"size:32" valid:"required,length(1|32),unix_user"`
	Addr     string       `valid:"optional"` // FIXME: to be removed in a future version in favor of URL
	User     string       `valid:"optional"` // FIXME: to be removed in a future version in favor of URL
	Password string       `valid:"optional"` // FIXME: to be removed in a future version in favor of URL
	URL      string       `valid:"optional"`
	SSHKey   *SSHKey      `gorm:"ForeignKey:SSHKeyID"` // SSHKey used to connect by the client
	SSHKeyID uint         `gorm:"index"`
	HostKey  []byte       `sql:"size:10000" valid:"optional"`
	Groups   []*HostGroup `gorm:"many2many:host_host_groups;"`
	Comment  string       `valid:"optional"`
}

// UserKey defines a user public key used by sshportal to identify the user
type UserKey struct {
	gorm.Model
	Key           []byte `sql:"size:10000" valid:"required,length(1|10000)"`
	AuthorizedKey string `sql:"size:10000" valid:"required,length(1|10000)"`
	UserID        uint   ``
	User          *User  `gorm:"ForeignKey:UserID"`
	Comment       string `valid:"optional"`
}

type UserRole struct {
	gorm.Model
	Name  string  `valid:"required,length(1|32),unix_user"`
	Users []*User `gorm:"many2many:user_user_roles"`
}

type User struct {
	// FIXME: use uuid for ID
	gorm.Model
	Roles       []*UserRole  `gorm:"many2many:user_user_roles"`
	Email       string       `valid:"required,email"`
	Name        string       `valid:"required,length(1|32),unix_user"`
	Keys        []*UserKey   `gorm:"ForeignKey:UserID"`
	Groups      []*UserGroup `gorm:"many2many:user_user_groups;"`
	Comment     string       `valid:"optional"`
	InviteToken string       `valid:"optional,length(10|60)"`
}

type UserGroup struct {
	gorm.Model
	Name    string  `valid:"required,length(1|32),unix_user"`
	Users   []*User `gorm:"many2many:user_user_groups;"`
	ACLs    []*ACL  `gorm:"many2many:user_group_acls;"`
	Comment string  `valid:"optional"`
}

type HostGroup struct {
	gorm.Model
	Name    string  `valid:"required,length(1|32),unix_user"`
	Hosts   []*Host `gorm:"many2many:host_host_groups;"`
	ACLs    []*ACL  `gorm:"many2many:host_group_acls;"`
	Comment string  `valid:"optional"`
}

type ACL struct {
	gorm.Model
	HostGroups  []*HostGroup `gorm:"many2many:host_group_acls;"`
	UserGroups  []*UserGroup `gorm:"many2many:user_group_acls;"`
	HostPattern string       `valid:"optional"`
	Action      string       `valid:"required"`
	Weight      uint         ``
	Comment     string       `valid:"optional"`
}

type Session struct {
	gorm.Model
	StoppedAt *time.Time `sql:"index" valid:"optional"`
	Status    string     `valid:"required"`
	User      *User      `gorm:"ForeignKey:UserID"`
	Host      *Host      `gorm:"ForeignKey:HostID"`
	UserID    uint       `valid:"optional"`
	HostID    uint       `valid:"optional"`
	ErrMsg    string     `valid:"optional"`
	Comment   string     `valid:"optional"`
}

type Event struct {
	gorm.Model
	Author   *User                  `gorm:"ForeignKey:AuthorID"`
	AuthorID uint                   `valid:"optional"`
	Domain   string                 `valid:"required"`
	Action   string                 `valid:"required"`
	Entity   string                 `valid:"optional"`
	Args     []byte                 `sql:"size:10000" valid:"optional,length(1|10000)" json:"-"`
	ArgsMap  map[string]interface{} `gorm:"-" json:"Args"`
}

type SessionStatus string

const (
	SessionStatusUnknown SessionStatus = "unknown"
	SessionStatusActive                = "active"
	SessionStatusClosed                = "closed"
)

type ACLAction string

const (
	ACLActionUnknown ACLAction = "unknown"
	ACLActionAllow             = "allow"
	ACLActionDeny              = "deny"
)

type BastionScheme string

const (
	BastionSchemeSSH    BastionScheme = "ssh"
	BastionSchemeTelnet               = "telnet"
)

func init() {
	unixUserRegexp := regexp.MustCompile("[a-z_][a-z0-9_-]*")

	govalidator.CustomTypeTagMap.Set("unix_user", govalidator.CustomTypeValidator(func(i interface{}, context interface{}) bool {
		name, ok := i.(string)
		if !ok {
			return false
		}
		return unixUserRegexp.MatchString(name)
	}))
}

// Host helpers

func ParseInputURL(input string) (*url.URL, error) {
	if !strings.Contains(input, "://") {
		input = "ssh://" + input
	}
	u, err := url.Parse(input)
	if err != nil {
		return nil, err
	}
	return u, nil
}
func (host *Host) DialAddr() string {
	return fmt.Sprintf("%s:%d", host.Hostname(), host.Port())
}
func (host *Host) String() string {
	if host.URL != "" {
		return host.URL
	} else if host.Addr != "" { // to be removed in a future version in favor of URL
		if host.Password != "" {
			return fmt.Sprintf("ssh://%s:%s@%s", host.User, strings.Repeat("*", 4), host.Addr)
		}
		return fmt.Sprintf("ssh://%s@%s", host.User, host.Addr)
	}
	return ""
}
func (host *Host) Scheme() BastionScheme {
	if host.URL != "" {
		u, err := url.Parse(host.URL)
		if err != nil {
			return BastionSchemeSSH
		}
		return BastionScheme(u.Scheme)
	} else if host.Addr != "" {
		return BastionSchemeSSH
	}
	return ""
}
func (host *Host) Hostname() string {
	if host.URL != "" {
		u, err := url.Parse(host.URL)
		if err != nil {
			return ""
		}
		return u.Hostname()
	} else if host.Addr != "" { // to be removed in a future version in favor of URL
		return strings.Split(host.Addr, ":")[0]
	}
	return ""
}
func (host *Host) Username() string {
	if host.URL != "" {
		u, err := url.Parse(host.URL)
		if err != nil {
			return "root"
		}
		if u.User != nil {
			return u.User.Username()
		}
	} else if host.User != "" { // to be removed in a future version in favor of URL
		return host.User
	}
	return "root"
}
func (host *Host) Passwd() string {
	if host.URL != "" {
		u, err := url.Parse(host.URL)
		if err != nil {
			return ""
		}
		if u.User != nil {
			password, _ := u.User.Password()
			return password
		}
	} else if host.Password != "" { // to be removed in a future version in favor of URL
		return host.Password
	}
	return ""
}
func (host *Host) Port() uint64 {
	var portString string
	if host.URL != "" {
		u, err := url.Parse(host.URL)
		if err != nil {
			goto defaultPort
		}
		portString = u.Port()
	} else if host.Addr != "" { // to be removed in a future version in favor of URL
		portString = strings.Split(host.Addr, ":")[1]
	}
	if portString != "" {
		port, err := strconv.ParseUint(portString, 10, 64)
		if err != nil {
			goto defaultPort
		}
		return port
	}
defaultPort:
	switch host.Scheme() {
	case BastionSchemeSSH:
		return 22
	case BastionSchemeTelnet:
		return 23
	default:
		return 0
	}
}
func HostsPreload(db *gorm.DB) *gorm.DB {
	return db.Preload("Groups").Preload("SSHKey")
}
func HostsByIdentifiers(db *gorm.DB, identifiers []string) *gorm.DB {
	return db.Where("id IN (?)", identifiers).Or("name IN (?)", identifiers)
}
func HostByName(db *gorm.DB, name string) (*Host, error) {
	var host Host
	db.Preload("SSHKey").Where("name = ?", name).Find(&host)
	if host.Name == "" {
		// FIXME: add available hosts
		return nil, fmt.Errorf("No such target: %q", name)
	}
	return &host, nil
}

func (host *Host) clientConfig(hk gossh.HostKeyCallback) (*gossh.ClientConfig, error) {
	config := gossh.ClientConfig{
		User:            host.Username(),
		HostKeyCallback: hk,
		Auth:            []gossh.AuthMethod{},
	}
	if host.SSHKey != nil {
		signer, err := gossh.ParsePrivateKey([]byte(host.SSHKey.PrivKey))
		if err != nil {
			return nil, err
		}
		config.Auth = append(config.Auth, gossh.PublicKeys(signer))
	}
	if host.Passwd() != "" {
		config.Auth = append(config.Auth, gossh.Password(host.Passwd()))
	}
	if len(config.Auth) == 0 {
		return nil, fmt.Errorf("no valid authentication method for host %q", host.Name)
	}
	return &config, nil
}

// SSHKey helpers

func SSHKeysPreload(db *gorm.DB) *gorm.DB {
	return db.Preload("Hosts")
}
func SSHKeysByIdentifiers(db *gorm.DB, identifiers []string) *gorm.DB {
	return db.Where("id IN (?)", identifiers).Or("name IN (?)", identifiers)
}

// HostGroup helpers

func HostGroupsPreload(db *gorm.DB) *gorm.DB {
	return db.Preload("ACLs").Preload("Hosts")
}
func HostGroupsByIdentifiers(db *gorm.DB, identifiers []string) *gorm.DB {
	return db.Where("id IN (?)", identifiers).Or("name IN (?)", identifiers)
}

// UserGroup helpers

func UserGroupsPreload(db *gorm.DB) *gorm.DB {
	return db.Preload("ACLs").Preload("Users")
}
func UserGroupsByIdentifiers(db *gorm.DB, identifiers []string) *gorm.DB {
	return db.Where("id IN (?)", identifiers).Or("name IN (?)", identifiers)
}

// User helpers

func UsersPreload(db *gorm.DB) *gorm.DB {
	return db.Preload("Groups").Preload("Keys").Preload("Roles")
}
func UsersByIdentifiers(db *gorm.DB, identifiers []string) *gorm.DB {
	return db.Where("id IN (?)", identifiers).Or("email IN (?)", identifiers).Or("name IN (?)", identifiers)
}
func (u *User) HasRole(name string) bool {
	for _, role := range u.Roles {
		if role.Name == name {
			return true
		}
	}
	return false
}
func (u *User) CheckRoles(names []string) error {
	for _, name := range names {
		if u.HasRole(name) {
			return nil
		}
	}
	return fmt.Errorf("you don't have permission to access this feature (requires any of these roles: '%s')", strings.Join(names, "', '"))
}

// ACL helpers

func ACLsPreload(db *gorm.DB) *gorm.DB {
	return db.Preload("UserGroups").Preload("HostGroups")
}
func ACLsByIdentifiers(db *gorm.DB, identifiers []string) *gorm.DB {
	return db.Where("id IN (?)", identifiers)
}

// UserKey helpers

func UserKeysPreload(db *gorm.DB) *gorm.DB {
	return db.Preload("User")
}
func UserKeysByIdentifiers(db *gorm.DB, identifiers []string) *gorm.DB {
	return db.Where("id IN (?)", identifiers)
}

// UserRole helpers

func UserRolesPreload(db *gorm.DB) *gorm.DB {
	return db.Preload("Users")
}
func UserRolesByIdentifiers(db *gorm.DB, identifiers []string) *gorm.DB {
	return db.Where("id IN (?)", identifiers).Or("name IN (?)", identifiers)
}

// Session helpers

func SessionsPreload(db *gorm.DB) *gorm.DB {
	return db.Preload("User").Preload("Host")
}
func SessionsByIdentifiers(db *gorm.DB, identifiers []string) *gorm.DB {
	return db.Where("id IN (?)", identifiers)
}

// Events helpers

func EventsPreload(db *gorm.DB) *gorm.DB {
	return db.Preload("Author")
}
func EventsByIdentifiers(db *gorm.DB, identifiers []string) *gorm.DB {
	return db.Where("id IN (?)", identifiers)
}

func NewEvent(domain, action string) *Event {
	return &Event{
		Domain:  domain,
		Action:  action,
		ArgsMap: map[string]interface{}{},
	}
}

func (e *Event) String() string {
	return fmt.Sprintf("%s %s %s %s", e.Domain, e.Action, e.Entity, string(e.Args))
}

func (e *Event) Log(db *gorm.DB) {
	if len(e.ArgsMap) > 0 {
		var err error
		if e.Args, err = json.Marshal(e.ArgsMap); err != nil {
			log.Printf("error: %v", err)
		}
	}
	log.Printf("info: %s", e)
	if err := db.Create(e).Error; err != nil {
		log.Printf("warning: %v", err)
	}
}

func (e *Event) SetAuthor(user *User) *Event {
	e.Author = user
	e.AuthorID = user.ID
	return e
}

func (e *Event) SetArg(name string, value interface{}) *Event {
	e.ArgsMap[name] = value
	return e
}
