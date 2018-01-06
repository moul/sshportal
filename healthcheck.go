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

// perform a healthcheck test without requiring an ssh client or an ssh key (used for Docker's HEALTHCHECK)
func healthcheck(addr string, wait, quiet bool) error {
	cfg := gossh.ClientConfig{
		User:            "healthcheck",
		HostKeyCallback: func(hostname string, remote net.Addr, key gossh.PublicKey) error { return nil },
		Auth:            []gossh.AuthMethod{gossh.Password("healthcheck")},
	}

	if wait {
		for {
			if err := healthcheckOnce(addr, cfg, quiet); err != nil {
				if !quiet {
					log.Printf("error: %v", err)
				}
				time.Sleep(time.Second)
				continue
			}
			return nil
		}
	}

	if err := healthcheckOnce(addr, cfg, quiet); err != nil {
		if quiet {
			return cli.NewExitError("", 1)
		}
		return err
	}
	return nil
}

func healthcheckOnce(addr string, config gossh.ClientConfig, quiet bool) error {
	client, err := gossh.Dial("tcp", addr, &config)
	if err != nil {
		return err
	}

	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer func() {
		if err := session.Close(); err != nil {
			if !quiet {
				log.Printf("failed to close session: %v", err)
			}
		}
	}()

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
