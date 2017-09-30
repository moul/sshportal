package main

import (
	"os"

	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
)

type Config struct {
	clientConfig *gossh.ClientConfig
	remoteAddr   string
}

func getConfig(s ssh.Session) (*Config, error) {
	// TODO: get the config from a database
	config := Config{
		remoteAddr: os.Getenv("SSH_ADDR"),
		clientConfig: &gossh.ClientConfig{
			User:            os.Getenv("SSH_USERNAME"),
			HostKeyCallback: gossh.InsecureIgnoreHostKey(), // TODO: show the remote host to the client + store it in db if approved
			Auth: []gossh.AuthMethod{
				gossh.Password(os.Getenv("SSH_PASSWORD")),
			},
		},
	}

	return &config, nil
}
