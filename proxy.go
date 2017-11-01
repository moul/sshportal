package main

import (
	"errors"
	"fmt"
	"io"
	"log"

	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
)

func proxy(s ssh.Session, host *Host) error {
	config, err := host.ClientConfig(s)
	if err != nil {
		return err
	}

	rconn, err := gossh.Dial("tcp", host.Addr, config)
	if err != nil {
		return err
	}
	defer rconn.Close()

	rch, rreqs, err := rconn.OpenChannel("session", []byte{})
	if err != nil {
		return err
	}

	log.Println("SSH Connectin established")
	return pipe(s.MaskedReqs(), rreqs, s, rch)
}

func pipe(lreqs, rreqs <-chan *gossh.Request, lch, rch gossh.Channel) error {
	defer func() {
		lch.Close()
		rch.Close()
	}()

	errch := make(chan error, 1)

	go func() {
		_, _ = io.Copy(lch, rch)
		errch <- errors.New("lch closed the connection")
	}()

	go func() {
		_, _ = io.Copy(rch, lch)
		errch <- errors.New("rch closed the connection")
	}()

	for {
		select {
		case req := <-lreqs: // forward ssh requests from local to remote
			if req == nil {
				return nil
			}
			b, err := rch.SendRequest(req.Type, req.WantReply, req.Payload)
			if err != nil {
				return err
			}
			req.Reply(b, nil)
		case req := <-rreqs: // forward ssh requests from remote to local
			if req == nil {
				return nil
			}
			b, err := lch.SendRequest(req.Type, req.WantReply, req.Payload)
			if err != nil {
				return err
			}
			req.Reply(b, nil)
		case err := <-errch:
			return err
		}
	}
	return nil
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
	if host.Key != nil {
		return nil, fmt.Errorf("auth by priv key is not yet implemented")
	}
	if len(config.Auth) == 0 {
		return nil, fmt.Errorf("no valid authentication method for host %q", host.Name)
	}
	return &config, nil
}
