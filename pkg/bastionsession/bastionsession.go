package bastionsession

import (
	"errors"
	"io"

	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
)

type Config struct {
	Addr         string
	ClientConfig *gossh.ClientConfig
}

func ChannelHandler(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context, config Config) error {
	if newChan.ChannelType() != "session" {
		newChan.Reject(gossh.UnknownChannelType, "unsupported channel type")
		return nil
	}
	lch, lreqs, err := newChan.Accept()
	// TODO: defer clean closer
	if err != nil {
		// TODO: trigger event callback
		return nil
	}

	// open client channel
	rconn, err := gossh.Dial("tcp", config.Addr, config.ClientConfig)
	if err != nil {
		return err
	}
	defer func() { _ = rconn.Close() }()
	rch, rreqs, err := rconn.OpenChannel("session", []byte{})
	if err != nil {
		return err
	}

	// pipe everything
	return pipe(lreqs, rreqs, lch, rch)
}

func pipe(lreqs, rreqs <-chan *gossh.Request, lch, rch gossh.Channel) error {
	defer func() {
		_ = lch.Close()
		_ = rch.Close()
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
			if err2 := req.Reply(b, nil); err2 != nil {
				return err2
			}
		case req := <-rreqs: // forward ssh requests from remote to local
			if req == nil {
				return nil
			}
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
