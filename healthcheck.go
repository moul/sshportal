package main

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"github.com/urfave/cli"
	gossh "golang.org/x/crypto/ssh"
)

const healthcheckTimeout = 5 * time.Second

// perform a healthcheck test without requiring an ssh client or an ssh key (used for Docker's HEALTHCHECK)
func healthcheck(addr string, wait, quiet bool) error {
	cfg := gossh.ClientConfig{
		User:            "healthcheck",
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Auth:            []gossh.AuthMethod{gossh.Password("healthcheck")},
	}

	for {
		err := healthcheckOnce(addr, cfg, healthcheckTimeout)
		if err == nil {
			return nil
		}
		if !wait {
			if quiet {
				return cli.NewExitError("", 1)
			}
			return err
		}
		if !quiet {
			log.Printf("error: %v", err)
		}
		time.Sleep(time.Second)
	}
}

func healthcheckOnce(addr string, config gossh.ClientConfig, timeout time.Duration) error {
	dialer := net.Dialer{Deadline: time.Now().Add(timeout)}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	// The dialer's deadline only covers connecting, so also bound the SSH exchange.
	if err := conn.SetDeadline(dialer.Deadline); err != nil {
		return err
	}
	sshConn, channels, requests, err := gossh.NewClientConn(conn, addr, &config)
	if err != nil {
		return err
	}
	client := gossh.NewClient(sshConn, channels, requests)

	session, err := client.NewSession()
	if err != nil {
		return err
	}

	var b bytes.Buffer
	session.Stdout = &b
	if err := session.Run(""); err != nil {
		return err
	}
	stdout := strings.TrimSpace(b.String())
	if stdout != "OK" {
		return fmt.Errorf("invalid stdout: %q expected 'OK'", stdout)
	}
	return nil
}
