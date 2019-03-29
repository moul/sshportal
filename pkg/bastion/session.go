package bastion // import "moul.io/sshportal/pkg/bastion"

import (
	"errors"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/moul/ssh"
	"github.com/sabban/bastion/pkg/logchannel"
	gossh "golang.org/x/crypto/ssh"
)

type sessionConfig struct {
	Addr         string
	Logs         string
	ClientConfig *gossh.ClientConfig
}

func multiChannelHandler(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context, configs []sessionConfig) error {
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
		// pipe everything
		return pipe(lreqs, rreqs, lch, rch, configs[len(configs)-1].Logs, user, newChan)
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
		// pipe everything
		return pipe(lreqs, rreqs, lch, rch, configs[len(configs)-1].Logs, user, newChan)
	default:
		if err := newChan.Reject(gossh.UnknownChannelType, "unsupported channel type"); err != nil {
			log.Printf("failed to reject chan: %v", err)
		}
		return nil
	}
}

func pipe(lreqs, rreqs <-chan *gossh.Request, lch, rch gossh.Channel, logsLocation string, user string, newChan gossh.NewChannel) error {
	defer func() {
		_ = lch.Close()
		_ = rch.Close()
	}()

	errch := make(chan error, 1)
	channeltype := newChan.ChannelType()

	filename := strings.Join([]string{logsLocation, "/", user, "-", channeltype, "-", time.Now().Format(time.RFC3339)}, "") // get user
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
	defer func() {
		_ = f.Close()
	}()

	if err != nil {
		log.Fatalf("error: %v", err)
	}

	log.Printf("Session %v is recorded in %v", channeltype, filename)
	if channeltype == "session" {
		wrappedlch := logchannel.New(lch, f)
		go func() {
			_, _ = io.Copy(wrappedlch, rch)
			errch <- errors.New("lch closed the connection")
		}()

		go func() {
			_, _ = io.Copy(rch, lch)
			errch <- errors.New("rch closed the connection")
		}()
	}
	if channeltype == "direct-tcpip" {
		d := logTunnelForwardData{}
		if err := gossh.Unmarshal(newChan.ExtraData(), &d); err != nil {
			return err
		}
		wrappedlch := newLogTunnel(lch, f, d.SourceHost)
		wrappedrch := newLogTunnel(rch, f, d.DestinationHost)
		go func() {
			_, _ = io.Copy(wrappedlch, rch)
			errch <- errors.New("lch closed the connection")
		}()

		go func() {
			_, _ = io.Copy(wrappedrch, lch)
			errch <- errors.New("rch closed the connection")
		}()
	}

	for {
		select {
		case req := <-lreqs: // forward ssh requests from local to remote
			if req == nil {
				return nil
			}
			b, err := rch.SendRequest(req.Type, req.WantReply, req.Payload)
			if req.Type == "exec" {
				wrappedlch := logchannel.New(lch, f)
				command := append(req.Payload, []byte("\n")...)
				if _, err := wrappedlch.LogWrite(command); err != nil {
					log.Printf("failed to write log: %v", err)
				}
			}

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
