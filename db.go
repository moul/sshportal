package main

import (
	"fmt"

	"github.com/gliderlabs/ssh"
	"github.com/jinzhu/gorm"
	gossh "golang.org/x/crypto/ssh"
)

type SSHKey struct {
	gorm.Model
	Type        string
	Fingerprint string
	PrivKey     []byte
	PubKey      []byte
}

type Host struct {
	gorm.Model
	Name        string
	Addr        string
	User        string
	Password    string
	Fingerprint string
	PrivKey     *SSHKey
}

type User struct {
	gorm.Model
	SSHKeys []SSHKey
}

func dbInit(db *gorm.DB) error {
	db.AutoMigrate(&User{})
	db.AutoMigrate(&SSHKey{})
	db.AutoMigrate(&Host{})
	return nil
}

func dbDemo(db *gorm.DB) error {
	var host1, host2, host3 Host
	db.FirstOrCreate(&host1, &Host{Name: "sdf", Addr: "sdf.org:22", User: "new"})
	db.FirstOrCreate(&host2, &Host{Name: "whoami", Addr: "whoami.filippo.io:22", User: "test"})
	db.FirstOrCreate(&host3, &Host{Name: "ssh-chat", Addr: "chat.shazow.net:22", User: "test", Fingerprint: "MD5:e5:d5:d1:75:90:38:42:f6:c7:03:d7:d0:56:7d:6a:db"})
	return nil
}

func RemoteHostFromSession(s ssh.Session, db *gorm.DB) (*Host, error) {
	var host Host
	db.Where("name = ?", s.User()).Find(&host)
	if host.Name == "" {
		// FIXME: add available hosts
		return nil, fmt.Errorf("No such target: %q", s.User())
	}
	return &host, nil
}

func (host *Host) ClientConfig(_ ssh.Session) (*gossh.ClientConfig, error) {
	config := gossh.ClientConfig{
		User:            host.User,
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Auth:            []gossh.AuthMethod{},
	}
	if host.Password != "" {
		config.Auth = append(config.Auth, gossh.Password(host.Password))
	}
	if host.PrivKey != nil {
		return nil, fmt.Errorf("auth by priv key is not yet implemented")
	}
	if len(config.Auth) == 0 {
		return nil, fmt.Errorf("no valid authentication method for host %q", host.Name)
	}
	return &config, nil
}
