package main

import (
	"fmt"
	"log"
	"net/url"
	"strings"

	"github.com/gliderlabs/ssh"
	"github.com/jinzhu/gorm"
)

type SSHKey struct {
	// FIXME: use uuid for ID
	gorm.Model
	Name        string // FIXME: govalidator: min length 3, alphanum
	Type        string
	Length      uint
	Fingerprint string
	PrivKey     string
	PubKey      string
	Comment     string
}

type Host struct {
	// FIXME: use uuid for ID
	gorm.Model
	Name        string // FIXME: govalidator: min length 3, alphanum
	Addr        string
	User        string
	Password    string
	SSHKey      *SSHKey
	SSHKeyID    uint   `gorm:"index"`
	Fingerprint string // FIXME: replace with hostkey ?
	Comment     string
}

type UserKey struct {
	gorm.Model
	Key     []byte
	UserID  uint
	User    *User
	Comment string
}

type User struct {
	// FIXME: use uuid for ID
	gorm.Model
	IsAdmin     bool
	Email       string // FIXME: govalidator: email
	Name        string // FIXME: govalidator: min length 3, alphanum
	Keys        []UserKey
	Comment     string
	InviteToken string
}

func dbInit(db *gorm.DB) error {
	db.AutoMigrate(&User{})
	db.AutoMigrate(&SSHKey{})
	db.AutoMigrate(&Host{})
	db.AutoMigrate(&UserKey{})
	db.Exec(`CREATE UNIQUE INDEX uix_keys_name   ON "ssh_keys"("name") WHERE ("deleted_at" IS NULL)`)
	db.Exec(`CREATE UNIQUE INDEX uix_hosts_name  ON "hosts"("name")    WHERE ("deleted_at" IS NULL)`)
	db.Exec(`CREATE UNIQUE INDEX uix_users_name  ON "users"("email")   WHERE ("deleted_at" IS NULL)`)

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

	// create admin user
	db.Table("users").Count(&count)
	if count == 0 {
		// if no admin, create an account for the first connection
		user := User{
			Name:        "Administrator",
			Email:       "admin@sshportal",
			Comment:     "created by sshportal",
			IsAdmin:     true,
			InviteToken: RandStringBytes(16),
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
	key, err := FindKeyByIdOrName(db, "default")
	if err != nil {
		return err
	}

	var host1, host2, host3 Host
	db.FirstOrCreate(&host1, &Host{Name: "sdf", Addr: "sdf.org:22", User: "new", SSHKeyID: key.ID})
	db.FirstOrCreate(&host2, &Host{Name: "whoami", Addr: "whoami.filippo.io:22", User: "test", SSHKeyID: key.ID})
	db.FirstOrCreate(&host3, &Host{Name: "ssh-chat", Addr: "chat.shazow.net:22", User: "test", SSHKeyID: key.ID, Fingerprint: "MD5:e5:d5:d1:75:90:38:42:f6:c7:03:d7:d0:56:7d:6a:db"})
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

func FindHostByIdOrName(db *gorm.DB, query string) (*Host, error) {
	var host Host
	if err := db.Preload("SSHKey").Where("id = ?", query).Or("name = ?", query).First(&host).Error; err != nil {
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

func FindKeyByIdOrName(db *gorm.DB, query string) (*SSHKey, error) {
	var key SSHKey
	if err := db.Where("id = ?", query).Or("name = ?", query).First(&key).Error; err != nil {
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

func FindUserByIdOrEmail(db *gorm.DB, query string) (*User, error) {
	var user User
	if err := db.Preload("Keys").Where("id = ?", query).Or("email = ?", query).First(&user).Error; err != nil {
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
