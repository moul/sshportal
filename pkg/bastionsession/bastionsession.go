package bastionsession

import (
	"errors"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/arkan/bastion/pkg/logchannel"
	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
)

type Config struct {
	Addr         string
	Logs         string
	ClientConfig *gossh.ClientConfig
}

func MultiChannelHandler(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context, configs []Config) error {
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

	var lastClient *gossh.Client

    // go through all the hops
	for _, config := range configs {
		var client *gossh.Client
		if lastClient == nil {
			client, err = gossh.Dial("tcp", config.Addr, config.ClientConfig)
		} else {
			rconn, err := lastClient.Dial("tcp", config.Addr)
			if err != nil {
				return err
			}
			ncc, chans, reqs, err := gossh.NewClientConn(rconn, config.Addr, config.ClientConfig)
			if err != nil {
				return err
			}
			client = gossh.NewClient(ncc, chans, reqs)
		}
		if err != nil {
			return err
		}
		defer func() { _ = client.Close() }()
		lastClient = client
	}
	rch, rreqs, err := lastClient.OpenChannel("session", []byte{})
	if err != nil {
		return err
	}
	user := conn.User()
	// pipe everything
	return pipe(lreqs, rreqs, lch, rch, configs[len(configs)-1].Logs, user)
}

func pipe(lreqs, rreqs <-chan *gossh.Request, lch, rch gossh.Channel, logsLocation string, user string) error {
	defer func() {
		_ = lch.Close()
		_ = rch.Close()
	}()

	errch := make(chan error, 1)
	file_name := strings.Join([]string{logsLocation, "/", user, "-", time.Now().Format(time.RFC3339)}, "") // get user
	f, err := os.OpenFile(file_name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	log.Printf("Session is recorded in %v", file_name)
	wrappedlch := logchannel.New(lch, f)
	go func() {
		_, _ = io.Copy(wrappedlch, rch)
		errch <- errors.New("lch closed the connection")
	}()

	defer f.Close()

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
