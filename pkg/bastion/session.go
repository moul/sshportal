package bastion // import "moul.io/sshportal/pkg/bastion"

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/pkg/errors"
	"github.com/sabban/bastion/pkg/logchannel"
	gossh "golang.org/x/crypto/ssh"
)

type sessionConfig struct {
	Addr         string
	LogsLocation string
	ClientConfig *gossh.ClientConfig
	LoggingMode  string
}

func multiChannelHandler(conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context, configs []sessionConfig, sessionID uint) error {
	var lastClient *gossh.Client
	switch newChan.ChannelType() {
	case "session":
		lch, lreqs, err := newChan.Accept()
		// TODO: defer clean closer
		if err != nil {
			// TODO: trigger event callback
			return nil
		}

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
				lch.Close() // fix #56
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
		actx := ctx.Value(authContextKey).(*authContext)
		username := actx.user.Name
		// pipe everything
		return pipe(lreqs, rreqs, lch, rch, configs[len(configs)-1], user, username, sessionID, newChan)
	case "direct-tcpip":
		lch, lreqs, err := newChan.Accept()
		// TODO: defer clean closer
		if err != nil {
			// TODO: trigger event callback
			return nil
		}

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
				lch.Close()
				return err
			}
			defer func() { _ = client.Close() }()
			lastClient = client
		}

		d := logTunnelForwardData{}
		if err := gossh.Unmarshal(newChan.ExtraData(), &d); err != nil {
			return err
		}
		rch, rreqs, err := lastClient.OpenChannel("direct-tcpip", newChan.ExtraData())
		if err != nil {
			return err
		}
		user := conn.User()
		actx := ctx.Value(authContextKey).(*authContext)
		username := actx.user.Name
		// pipe everything
		return pipe(lreqs, rreqs, lch, rch, configs[len(configs)-1], user, username, sessionID, newChan)
	default:
		if err := newChan.Reject(gossh.UnknownChannelType, "unsupported channel type"); err != nil {
			log.Printf("failed to reject chan: %v", err)
		}
		return nil
	}
}

func pipe(lreqs, rreqs <-chan *gossh.Request, lch, rch gossh.Channel, sessConfig sessionConfig, user string, username string, sessionID uint, newChan gossh.NewChannel) error {
	defer func() {
		_ = lch.Close()
		_ = rch.Close()
	}()

	errch := make(chan error, 1)
	quit := make(chan string, 1)
	channeltype := newChan.ChannelType()

	var logWriter io.WriteCloser = newDiscardWriteCloser()
	if sessConfig.LoggingMode != "disabled" {
		filename := filepath.Join(sessConfig.LogsLocation, fmt.Sprintf("%s-%s-%s-%d-%s", user, username, channeltype, sessionID, time.Now().Format(time.RFC3339)))
		f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0440)
		if err != nil {
			return errors.Wrap(err, "open log file")
		}
		defer func() {
			_ = f.Close()
		}()
		log.Printf("Session %v is recorded in %v", channeltype, filename)
		logWriter = f
	}

	if channeltype == "session" {
		switch sessConfig.LoggingMode {
		case "input":
			wrappedrch := logchannel.New(rch, logWriter)
			go func(quit chan string) {
				_, _ = io.Copy(lch, rch)
				quit <- "rch"
			}(quit)
			go func(quit chan string) {
				_, _ = io.Copy(wrappedrch, lch)
				quit <- "lch"
			}(quit)
		default: // everything, disabled
			wrappedlch := logchannel.New(lch, logWriter)
			go func(quit chan string) {
				_, _ = io.Copy(wrappedlch, rch)
				quit <- "rch"
			}(quit)
			go func(quit chan string) {
				_, _ = io.Copy(rch, lch)
				quit <- "lch"
			}(quit)
		}
	}
	if channeltype == "direct-tcpip" {
		d := logTunnelForwardData{}
		if err := gossh.Unmarshal(newChan.ExtraData(), &d); err != nil {
			return err
		}
		wrappedlch := newLogTunnel(lch, logWriter, d.SourceHost)
		wrappedrch := newLogTunnel(rch, logWriter, d.DestinationHost)
		go func(quit chan string) {
			_, _ = io.Copy(wrappedlch, rch)
			quit <- "rch"
		}(quit)

		go func(quit chan string) {
			_, _ = io.Copy(wrappedrch, lch)
			quit <- "lch"
		}(quit)
	}

	go func(quit chan string) {
		for req := range lreqs {
			b, err := rch.SendRequest(req.Type, req.WantReply, req.Payload)
			if req.Type == "exec" {
				wrappedlch := logchannel.New(lch, logWriter)
				req.Payload = append(req.Payload, []byte("\n")...)
				if _, err := wrappedlch.LogWrite(req.Payload); err != nil {
					log.Printf("failed to write log: %v", err)
				}
			}

			if err != nil {
				errch <- err
			}
			if err2 := req.Reply(b, nil); err2 != nil {
				errch <- err2
			}
		}
		quit <- "lreqs"
	}(quit)

	go func(quit chan string) {
		for req := range rreqs {
			b, err := lch.SendRequest(req.Type, req.WantReply, req.Payload)
			if err != nil {
				errch <- err
			}
			if err2 := req.Reply(b, nil); err2 != nil {
				errch <- err2
			}
		}
		quit <- "rreqs"
	}(quit)

	lchEOF, rchEOF, lchClosed, rchClosed := false, false, false, false
	for {
		select {
		case err := <-errch:
			return err
		case q := <-quit:
			switch q {
			case "lch":
				lchEOF = true
				_ = rch.CloseWrite()
			case "rch":
				rchEOF = true
				_ = lch.CloseWrite()
			case "lreqs":
				lchClosed = true
			case "rreqs":
				rchClosed = true
			}

			if lchEOF && lchClosed && !rchClosed {
				rch.Close()
			}

			if rchEOF && rchClosed && !lchClosed {
				lch.Close()
			}

			if lchEOF && rchEOF && lchClosed && rchClosed {
				return nil
			}
		}
	}
}

func newDiscardWriteCloser() io.WriteCloser { return &discardWriteCloser{ioutil.Discard} }

type discardWriteCloser struct {
	io.Writer
}

func (discardWriteCloser) Close() error {
	return nil
}
