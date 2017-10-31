package main

import (
	"fmt"

	"github.com/gliderlabs/ssh"
	"github.com/jinzhu/gorm"
	gossh "golang.org/x/crypto/ssh"
)

type Config struct {
	clientConfig *gossh.ClientConfig
	remoteAddr   string
}

func getCurrentUser(s ssh.Session, db *gorm.DB) (*User, error) {
	return &User{}, nil
}

func getConfig(s ssh.Session, db *gorm.DB) (*Config, error) {
	var host Host
	db.Where("name = ?", s.User()).Find(&host)
	if host.Name == "" {
		// FIXME: add available hosts
		return nil, fmt.Errorf("No such target: %q", s.User())
	}

	config := Config{
		remoteAddr: host.Addr,
		clientConfig: &gossh.ClientConfig{
			User:            host.User,
			HostKeyCallback: gossh.InsecureIgnoreHostKey(),
			Auth:            []gossh.AuthMethod{},
		},
	}
	if host.Password != "" {
		config.clientConfig.Auth = append(config.clientConfig.Auth, gossh.Password(host.Password))
	}
	if host.PrivKey != nil {
		return nil, fmt.Errorf("auth by priv key is not yet implemented")
	}
	if len(config.clientConfig.Auth) == 0 {
		return nil, fmt.Errorf("no valid authentication method for host %q", s.User())
	}

	return &config, nil
}
