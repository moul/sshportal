package bastionsession

import (
	"errors"
	"io"
	"strings"
	"time"
	"os"
	"log"
	
	"github.com/gliderlabs/ssh"
	"github.com/arkan/bastion/pkg/logchannel"
	"github.com/moul/sshportal/pkg/logtunnel"
	gossh "golang.org/x/crypto/ssh"
)


type ForwardData struct {
	DestinationHost string
	DestinationPort uint32
	SourceHost string
	SourcePort uint32
}

type Config struct {
	Addr         string
	Logs         string
	ClientConfig *gossh.ClientConfig
}

func ChannelHandler(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context, config Config) error {
	switch newChan.ChannelType() {
	case "session" :
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
		return pipe(lreqs, rreqs, lch, rch, config.Logs, user, newChan)
	case "direct-tcpip":
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
		d := logtunnel.ForwardData{}
		if err := gossh.Unmarshal(newChan.ExtraData(), &d); err != nil {
			return err
		}
		rch, rreqs, err := rconn.OpenChannel("direct-tcpip", newChan.ExtraData())
		if err != nil {
			return err
		}
		user := conn.User()
		// pipe everything
		return pipe(lreqs, rreqs, lch, rch, config.Logs, user, newChan)
	default:
		newChan.Reject(gossh.UnknownChannelType, "unsupported channel type")
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

	file_name := strings.Join([]string{logsLocation, "/", user, "-", channeltype, "-", time.Now().Format(time.RFC3339)}, "") // get user
	f, err := os.OpenFile(file_name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0640)
	defer f.Close()

	if err != nil {
		log.Fatalf("error: %v", err)
	} 

	log.Printf("Session %v is recorded in %v", channeltype, file_name)
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
		d := logtunnel.ForwardData{}
		if err := gossh.Unmarshal(newChan.ExtraData(), &d); err != nil {
			return err
		}
		wrappedlch := logtunnel.New(lch, f, d.SourceHost)
		wrappedrch := logtunnel.New(rch, f, d.DestinationHost)
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
