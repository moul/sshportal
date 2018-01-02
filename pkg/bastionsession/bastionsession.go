package bastionsession

import (
	"errors"
	"io"
	"strings"
	"time"
	"os"
	
	"github.com/gliderlabs/ssh"
	"github.com/sabban/sshportal/pkg/logchannel"
	gossh "golang.org/x/crypto/ssh"
)

type Config struct {
	Addr         string
	Logs         string
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
	user := conn.User()
	// pipe everything
	return pipe(lreqs, rreqs, lch, rch, config.Logs, user)
}

func pipe(lreqs, rreqs <-chan *gossh.Request, lch, rch gossh.Channel, logs_location string, user string) error {
	defer func() {
		_ = lch.Close()
		_ = rch.Close()
	}()

	errch := make(chan error, 1)
	file_name := strings.Join([]string{logs_location, "/", user, "-", time.Now().Format("RFC3339")}, "") // get user
	f, err := os.OpenFile(file_name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
	if err != nil {
		errch <- errors.New("Opening session file" + file_name + "failed.")
	}
	defer f.Close()
	wrappedlch := logchannel.New(lch, f) 
	
	go func() {
		_, _ = io.Copy(wrappedlch, rch)
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
