package main

import (
	"errors"
	"io"
	"log"

	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
)

func proxy(s ssh.Session, config *Config) error {
	rconn, err := gossh.Dial("tcp", config.remoteAddr, config.clientConfig)
	if err != nil {
		return err
	}
	defer rconn.Close()

	rch, rreqs, err := rconn.OpenChannel("session", []byte{})
	if err != nil {
		return err
	}

	log.Println("SSH Connectin established")
	lreqs := make(chan *gossh.Request, 1)
	defer close(lreqs)

	return pipe(lreqs, rreqs, s, rch)
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

	go func() {
		// FIXME: find a way to get the client requests
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
