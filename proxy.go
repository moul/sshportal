package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
)

func proxy(s ssh.Session, host *Host, hk gossh.HostKeyCallback) error {
	config, err := host.clientConfig(s, hk)
	if err != nil {
		return err
	}

	rconn, err := gossh.Dial("tcp", host.Addr, config)
	if err != nil {
		return err
	}
	defer func() { _ = rconn.Close() }()

	rch, rreqs, err := rconn.OpenChannel("session", []byte{})
	if err != nil {
		return err
	}

	log.Println("SSH Connection established")
	return pipe(s.MaskedReqs(), rreqs, s, rch)
}

func pipe(lreqs, rreqs <-chan *gossh.Request, lch, rch gossh.Channel) error {
	defer func() {
		_ = lch.Close()
		_ = rch.Close()
	}()

	errch := make(chan error, 1)
	file, err := os.Create("/tmp/test")
	go func() {
		w := io.MultiWriter(&lch, &file)
		_, _ = io.Copy(w, rch)
		errch <- errors.New("lch closed the connection")
	}()

	go func() {
		w := io.MultiWriter(&rch, &file)
		_, _ = io.Copy(w, lch)
		errch <- errors.New("rch closed the connection")
	}()

	for {
		select {
		case req := <-lreqs: // forward ssh requests from local to remote
			if req == nil {
				return nil
			}
			log.Println("%s\n", req.Payload)
			b, err := rch.SendRequest(req.Type, req.WantReply, req.Payload)
			if err != nil {
				return err
			}
			if err2 := req.Reply(b, nil); err2 != nil {
				return err2
			}
		case req := <-rreqs: // forward ssh requests from remote to local
			if req == nil {
				return nil
			}
			log.Println("%s\n", req.Payload)
			b, err := lch.SendRequest(req.Type, req.WantReply, req.Payload)
			if err != nil {
				return err
			}
			if err2 := req.Reply(b, nil); err2 != nil {
				return err2
			}
		case err := <-errch:
			return err
		}
	}
}

func (host *Host) clientConfig(_ ssh.Session, hk gossh.HostKeyCallback) (*gossh.ClientConfig, error) {
	config := gossh.ClientConfig{
		User:            host.User,
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
	if host.Password != "" {
		config.Auth = append(config.Auth, gossh.Password(host.Password))
	}
	if len(config.Auth) == 0 {
		return nil, fmt.Errorf("no valid authentication method for host %q", host.Name)
	}
	return &config, nil
}
