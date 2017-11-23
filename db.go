package main

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/gliderlabs/ssh"
	"github.com/jinzhu/gorm"
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
	Date       time.Time    `json:"date"`
}

type Setting struct {
	gorm.Model
	Name  string `valid:"required"`
	Value string `valid:"required"`
}

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
	Name        string       `gorm:"size:32" valid:"required,length(1|32),unix_user"`
	Addr        string       `valid:"required"`
	User        string       `valid:"optional"`
	Password    string       `valid:"optional"`
	SSHKey      *SSHKey      `gorm:"ForeignKey:SSHKeyID"`
	SSHKeyID    uint         `gorm:"index"`
	Groups      []*HostGroup `gorm:"many2many:host_host_groups;"`
	Fingerprint string       `valid:"optional"` // FIXME: replace with hostKey ?
	Comment     string       `valid:"optional"`
}

type UserKey struct {
	gorm.Model
	Key     []byte `sql:"size:10000" valid:"required,length(1|10000)"`
	UserID  uint   ``
	User    *User  `gorm:"ForeignKey:UserID"`
	Comment string `valid:"optional"`
}

type User struct {
	// FIXME: use uuid for ID
	gorm.Model
	IsAdmin     bool
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

func RemoteHostFromSession(s ssh.Session, db *gorm.DB) (*Host, error) {
	var host Host
	db.Preload("SSHKey").Where("name = ?", s.User()).Find(&host)
	if host.Name == "" {
		// FIXME: add available hosts
		return nil, fmt.Errorf("No such target: %q", s.User())
	}
	return &host, nil
}

func (host *Host) URL() string {
	return fmt.Sprintf("%s@%s", host.User, host.Addr)
}

func NewHostFromURL(rawurl string) (*Host, error) {
	if !strings.Contains(rawurl, "://") {
		rawurl = "ssh://" + rawurl
	}
	u, err := url.Parse(rawurl)
	if err != nil {
		return nil, err
	}
	host := Host{Addr: u.Host}
	if !strings.Contains(host.Addr, ":") {
		host.Addr += ":22" // add port if not present
	}
	host.User = "root" // default username
	if u.User != nil {
		password, _ := u.User.Password()
		host.Password = password
		host.User = u.User.Username()
	}
	return &host, nil
}

func (host *Host) Hostname() string {
	return strings.Split(host.Addr, ":")[0]
}

// Host helpers
func HostsPreload(db *gorm.DB) *gorm.DB {
	return db.Preload("Groups").Preload("SSHKey")
}
func HostsByIdentifiers(db *gorm.DB, identifiers []string) *gorm.DB {
	return db.Where("id IN (?)", identifiers).Or("name IN (?)", identifiers)
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

// UserGroup heleprs
func UserGroupsPreload(db *gorm.DB) *gorm.DB {
	return db.Preload("ACLs").Preload("Users")
}
func UserGroupsByIdentifiers(db *gorm.DB, identifiers []string) *gorm.DB {
	return db.Where("id IN (?)", identifiers).Or("name IN (?)", identifiers)
}

// User helpers
func UsersPreload(db *gorm.DB) *gorm.DB {
	return db.Preload("Groups").Preload("Keys")
}
func UsersByIdentifiers(db *gorm.DB, identifiers []string) *gorm.DB {
	return db.Where("id IN (?)", identifiers).Or("email IN (?)", identifiers)
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
