package main

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/gliderlabs/ssh"
	"github.com/jinzhu/gorm"
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

type Config struct {
	SSHKeys    []*SSHKey    `json:"keys"`
	Hosts      []*Host      `json:"hosts"`
	UserKeys   []*UserKey   `json:"user_keys"`
	Users      []*User      `json:"users"`
	UserGroups []*UserGroup `json:"user_groups"`
	HostGroups []*HostGroup `json:"host_groups"`
	ACLs       []*ACL       `json:"acls"`
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
	Addr        string       `gorm:"required,host"`
	User        string       `gorm:"optional"`
	Password    string       `gorm:"optional"`
	SSHKey      *SSHKey      `gorm:"ForeignKey:SSHKeyID"`
	SSHKeyID    uint         `gorm:"index"`
	Groups      []*HostGroup `gorm:"many2many:host_host_groups;"`
	Fingerprint string       `gorm:"optional"` // FIXME: replace with hostKey ?
	Comment     string       `gorm:"optional"`
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

func dbInit(db *gorm.DB) error {
	// version checking
	db.AutoMigrate(&Setting{})
	db.Exec(`CREATE UNIQUE INDEX uix_settings_name ON "settings"("name") WHERE ("deleted_at" IS NULL)`)
	var versionSetting Setting
	if db.Where("name = ?", "version").First(&versionSetting).RecordNotFound() {
		db.Create(&Setting{Name: "version", Value: VERSION})
	}
	if versionSetting.Value != VERSION {
		log.Printf("database is not sync, applying migrations.\n")
		// other models
		db.AutoMigrate(&User{})
		db.AutoMigrate(&SSHKey{})
		db.AutoMigrate(&Host{})
		db.AutoMigrate(&UserKey{})
		db.AutoMigrate(&UserGroup{})
		db.AutoMigrate(&HostGroup{})
		db.AutoMigrate(&ACL{})
		// FIXME: check if indexes exist to avoid gorm warns
		db.Exec(`CREATE UNIQUE INDEX uix_keys_name        ON "ssh_keys"("name")      WHERE ("deleted_at" IS NULL)`)
		db.Exec(`CREATE UNIQUE INDEX uix_hosts_name       ON "hosts"("name")         WHERE ("deleted_at" IS NULL)`)
		db.Exec(`CREATE UNIQUE INDEX uix_users_name       ON "users"("email")        WHERE ("deleted_at" IS NULL)`)
		db.Exec(`CREATE UNIQUE INDEX uix_usergroups_name  ON "user_groups"("name")   WHERE ("deleted_at" IS NULL)`)
		db.Exec(`CREATE UNIQUE INDEX uix_hostgroups_name  ON "host_groups"("name")   WHERE ("deleted_at" IS NULL)`)
		versionSetting.Value = VERSION
		if err := db.Update(&versionSetting).Error; err != nil {
			return err
		}
	}

	// create default ssh key
	var count uint
	if err := db.Table("ssh_keys").Where("name = ?", "default").Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		key, err := NewSSHKey("rsa", 2048)
		if err != nil {
			return err
		}
		key.Name = "default"
		key.Comment = "created by sshportal"
		if err := db.Create(&key).Error; err != nil {
			return err
		}
	}

	// create default host group
	if err := db.Table("host_groups").Where("name = ?", "default").Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		hostGroup := HostGroup{
			Name:    "default",
			Comment: "created by sshportal",
		}
		if err := db.Create(&hostGroup).Error; err != nil {
			return err
		}
	}

	// create default user group
	if err := db.Table("user_groups").Where("name = ?", "default").Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		userGroup := UserGroup{
			Name:    "default",
			Comment: "created by sshportal",
		}
		if err := db.Create(&userGroup).Error; err != nil {
			return err
		}
	}

	// create default acl
	if err := db.Table("acls").Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		var defaultUserGroup UserGroup
		db.Where("name = ?", "default").First(&defaultUserGroup)
		var defaultHostGroup HostGroup
		db.Where("name = ?", "default").First(&defaultHostGroup)
		acl := ACL{
			UserGroups: []*UserGroup{&defaultUserGroup},
			HostGroups: []*HostGroup{&defaultHostGroup},
			Action:     "allow",
			//HostPattern: "",
			//Weight:      0,
			Comment: "created by sshportal",
		}
		if err := db.Create(&acl).Error; err != nil {
			return err
		}
	}

	// create admin user
	var defaultUserGroup UserGroup
	db.Where("name = ?", "default").First(&defaultUserGroup)
	db.Table("users").Count(&count)
	if count == 0 {
		// if no admin, create an account for the first connection
		inviteToken := RandStringBytes(16)
		if os.Getenv("SSHPORTAL_DEFAULT_ADMIN_INVITE_TOKEN") != "" {
			inviteToken = os.Getenv("SSHPORTAL_DEFAULT_ADMIN_INVITE_TOKEN")
		}
		user := User{
			Name:        "Administrator",
			Email:       "admin@sshportal",
			Comment:     "created by sshportal",
			IsAdmin:     true,
			InviteToken: inviteToken,
			Groups:      []*UserGroup{&defaultUserGroup},
		}
		db.Create(&user)
		log.Printf("Admin user created, use the user 'invite:%s' to associate a public key with this account", user.InviteToken)
	}

	// create host ssh key
	if err := db.Table("ssh_keys").Where("name = ?", "host").Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		key, err := NewSSHKey("rsa", 2048)
		if err != nil {
			return err
		}
		key.Name = "host"
		key.Comment = "created by sshportal"
		if err := db.Create(&key).Error; err != nil {
			return err
		}
	}
	return nil
}

func dbDemo(db *gorm.DB) error {
	hostGroup, err := FindHostGroupByIdOrName(db, "default")
	if err != nil {
		return err
	}

	key, err := FindKeyByIdOrName(db, "default")
	if err != nil {
		return err
	}

	var (
		host1 = Host{Name: "sdf", Addr: "sdf.org:22", User: "new", SSHKeyID: key.ID, Groups: []*HostGroup{hostGroup}}
		host2 = Host{Name: "whoami", Addr: "whoami.filippo.io:22", User: "test", SSHKeyID: key.ID, Groups: []*HostGroup{hostGroup}}
		host3 = Host{Name: "ssh-chat", Addr: "chat.shazow.net:22", User: "test", SSHKeyID: key.ID, Fingerprint: "MD5:e5:d5:d1:75:90:38:42:f6:c7:03:d7:d0:56:7d:6a:db", Groups: []*HostGroup{hostGroup}}
	)

	// FIXME: check if hosts exist to avoid `UNIQUE constraint` error
	db.Create(&host1)
	db.Create(&host2)
	db.Create(&host3)
	return nil
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

func FindHostByIdOrName(db *gorm.DB, query string) (*Host, error) {
	var host Host
	if err := db.Preload("Groups").Preload("SSHKey").Where("id = ?", query).Or("name = ?", query).First(&host).Error; err != nil {
		return nil, err
	}
	return &host, nil
}
func FindHostsByIdOrName(db *gorm.DB, queries []string) ([]*Host, error) {
	var hosts []*Host
	for _, query := range queries {
		host, err := FindHostByIdOrName(db, query)
		if err != nil {
			return nil, err
		}
		hosts = append(hosts, host)
	}
	return hosts, nil
}

// SSHKey helpers

func FindKeyByIdOrName(db *gorm.DB, query string) (*SSHKey, error) {
	var key SSHKey
	if err := db.Preload("Hosts").Where("id = ?", query).Or("name = ?", query).First(&key).Error; err != nil {
		return nil, err
	}
	return &key, nil
}
func FindKeysByIdOrName(db *gorm.DB, queries []string) ([]*SSHKey, error) {
	var keys []*SSHKey
	for _, query := range queries {
		key, err := FindKeyByIdOrName(db, query)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, nil
}

// HostGroup helpers

func FindHostGroupByIdOrName(db *gorm.DB, query string) (*HostGroup, error) {
	var hostGroup HostGroup
	if err := db.Preload("ACLs").Preload("Hosts").Where("id = ?", query).Or("name = ?", query).First(&hostGroup).Error; err != nil {
		return nil, err
	}
	return &hostGroup, nil
}
func FindHostGroupsByIdOrName(db *gorm.DB, queries []string) ([]*HostGroup, error) {
	var hostGroups []*HostGroup
	for _, query := range queries {
		hostGroup, err := FindHostGroupByIdOrName(db, query)
		if err != nil {
			return nil, err
		}
		hostGroups = append(hostGroups, hostGroup)
	}
	return hostGroups, nil
}

// UserGroup heleprs

func FindUserGroupByIdOrName(db *gorm.DB, query string) (*UserGroup, error) {
	var userGroup UserGroup
	if err := db.Preload("ACLs").Preload("Users").Where("id = ?", query).Or("name = ?", query).First(&userGroup).Error; err != nil {
		return nil, err
	}
	return &userGroup, nil
}
func FindUserGroupsByIdOrName(db *gorm.DB, queries []string) ([]*UserGroup, error) {
	var userGroups []*UserGroup
	for _, query := range queries {
		userGroup, err := FindUserGroupByIdOrName(db, query)
		if err != nil {
			return nil, err
		}
		userGroups = append(userGroups, userGroup)
	}
	return userGroups, nil
}

// User helpers

func FindUserByIdOrEmail(db *gorm.DB, query string) (*User, error) {
	var user User
	if err := db.Preload("Groups").Preload("Keys").Where("id = ?", query).Or("email = ?", query).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}
func FindUsersByIdOrEmail(db *gorm.DB, queries []string) ([]*User, error) {
	var users []*User
	for _, query := range queries {
		user, err := FindUserByIdOrEmail(db, query)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}

// ACL helpers

func FindACLById(db *gorm.DB, query string) (*ACL, error) {
	var acl ACL
	if err := db.Preload("UserGroups").Preload("HostGroups").Where("id = ?", query).First(&acl).Error; err != nil {
		return nil, err
	}
	return &acl, nil
}
func FindACLsById(db *gorm.DB, queries []string) ([]*ACL, error) {
	var acls []*ACL
	for _, query := range queries {
		acl, err := FindACLById(db, query)
		if err != nil {
			return nil, err
		}
		acls = append(acls, acl)
	}
	return acls, nil
}

// UserKey helpers

func FindUserkeyById(db *gorm.DB, query string) (*UserKey, error) {
	var userkey UserKey
	if err := db.Preload("User").Where("id = ?", query).First(&userkey).Error; err != nil {
		return nil, err
	}
	return &userkey, nil
}
func FindUserkeysById(db *gorm.DB, queries []string) ([]*UserKey, error) {
	var userkeys []*UserKey
	for _, query := range queries {
		userkey, err := FindUserkeyById(db, query)
		if err != nil {
			return nil, err
		}
		userkeys = append(userkeys, userkey)
	}
	return userkeys, nil
}
