package bastionsession

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/anmitsu/go-shlex"
	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
)

// maxSigBufSize is how many signals will be buffered
// when there is no signal channel specified
const maxSigBufSize = 128

func ChannelHandler(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context, handler ssh.Handler) {
	if newChan.ChannelType() != "session" {
		newChan.Reject(gossh.UnknownChannelType, "unsupported channel type")
		return
	}
	ch, reqs, err := newChan.Accept()
	if err != nil {
		// TODO: trigger event callback
		return
	}
	if handler == nil {
		handler = srv.Handler
	}
	sess := &session{
		Channel:    ch,
		conn:       conn,
		handler:    handler,
		ptyCb:      srv.PtyCallback,
		maskedReqs: make(chan *gossh.Request, 5),
		ctx:        ctx,
	}
	sess.ctx.SetValue("masked-reqs", sess.maskedReqs)
	//ssh.DefaultChannelHandler(srv, conn, ch, ctx)
	//return
	sess.handleRequests(reqs)
}

type session struct {
	sync.Mutex
	gossh.Channel
	conn       *gossh.ServerConn
	handler    ssh.Handler
	handled    bool
	exited     bool
	pty        *ssh.Pty
	winch      chan ssh.Window
	env        []string
	ptyCb      ssh.PtyCallback
	cmd        []string
	ctx        ssh.Context
	sigCh      chan<- ssh.Signal
	sigBuf     []ssh.Signal
	maskedReqs chan *gossh.Request
}

func (sess *session) Write(p []byte) (n int, err error) {
	if sess.pty != nil {
		m := len(p)
		// normalize \n to \r\n when pty is accepted.
		// this is a hardcoded shortcut since we don't support terminal modes.
		p = bytes.Replace(p, []byte{'\n'}, []byte{'\r', '\n'}, -1)
		p = bytes.Replace(p, []byte{'\r', '\r', '\n'}, []byte{'\r', '\n'}, -1)
		n, err = sess.Channel.Write(p)
		if n > m {
			n = m
		}
		return
	}
	return sess.Channel.Write(p)
}

func (sess *session) PublicKey() ssh.PublicKey {
	sessionkey := sess.ctx.Value(ssh.ContextKeyPublicKey)
	if sessionkey == nil {
		return nil
	}
	return sessionkey.(ssh.PublicKey)
}

func (sess *session) Permissions() ssh.Permissions {
	// use context permissions because its properly
	// wrapped and easier to dereference
	perms := sess.ctx.Value(ssh.ContextKeyPermissions).(*ssh.Permissions)
	return *perms
}

func (sess *session) Context() context.Context {
	return sess.ctx
}

func (sess *session) Exit(code int) error {
	sess.Lock()
	defer sess.Unlock()

	if sess.exited {
		return errors.New("Session.Exit called multiple times")
	}
	sess.exited = true

	status := struct{ Status uint32 }{uint32(code)}
	_, err := sess.SendRequest("exit-status", false, gossh.Marshal(&status))
	if err != nil {
		return err
	}

	close(sess.maskedReqs)

	return sess.Close()
}

func (sess *session) User() string {
	return sess.conn.User()
}

func (sess *session) RemoteAddr() net.Addr {
	return sess.conn.RemoteAddr()
}

func (sess *session) LocalAddr() net.Addr {
	return sess.conn.LocalAddr()
}

func (sess *session) Environ() []string {
	return append([]string(nil), sess.env...)
}

func (sess *session) Command() []string {
	return append([]string(nil), sess.cmd...)
}

func (sess *session) Pty() (ssh.Pty, <-chan ssh.Window, bool) {
	if sess.pty != nil {
		return *sess.pty, sess.winch, true
	}
	return ssh.Pty{}, sess.winch, false
}

func (sess *session) MaskedReqs() chan *gossh.Request {
	return sess.maskedReqs
}

func (sess *session) Signals(c chan<- ssh.Signal) {
	sess.Lock()
	defer sess.Unlock()
	sess.sigCh = c
	if len(sess.sigBuf) > 0 {
		go func() {
			for _, sig := range sess.sigBuf {
				sess.sigCh <- sig
			}
		}()
	}
}

func (sess *session) handleRequests(reqs <-chan *gossh.Request) {
	for req := range reqs {
		addToMaskedReqs := true
		switch req.Type {
		case "shell", "exec":
			if sess.handled {
				req.Reply(false, nil)
				addToMaskedReqs = false
				continue
			}
			sess.handled = true
			// req.Reply(true, nil) // let the proxy reply

			var payload = struct{ Value string }{}
			gossh.Unmarshal(req.Payload, &payload)
			sess.cmd, _ = shlex.Split(payload.Value, true)
			go func() {
				sess.handler(sess)
				sess.Exit(0)
			}()
		case "env":
			if sess.handled {
				req.Reply(false, nil)
				continue
			}
			var kv struct{ Key, Value string }
			gossh.Unmarshal(req.Payload, &kv)
			sess.env = append(sess.env, fmt.Sprintf("%s=%s", kv.Key, kv.Value))
			req.Reply(true, nil)
		case "signal":
			var payload struct{ Signal string }
			gossh.Unmarshal(req.Payload, &payload)
			sess.Lock()
			if sess.sigCh != nil {
				sess.sigCh <- ssh.Signal(payload.Signal)
			} else {
				if len(sess.sigBuf) < maxSigBufSize {
					sess.sigBuf = append(sess.sigBuf, ssh.Signal(payload.Signal))
				}
			}
			sess.Unlock()
		case "pty-req":
			if sess.handled || sess.pty != nil {
				req.Reply(false, nil)
				addToMaskedReqs = false
				continue
			}
			ptyReq, ok := parsePtyRequest(req.Payload)
			if !ok {
				req.Reply(false, nil)
				addToMaskedReqs = false
				continue
			}
			if sess.ptyCb != nil {
				ok := sess.ptyCb(sess.ctx, ptyReq)
				if !ok {
					req.Reply(false, nil)
					addToMaskedReqs = false
					continue
				}
			}
			sess.pty = &ptyReq
			sess.winch = make(chan ssh.Window, 1)
			sess.winch <- ptyReq.Window
			defer func() {
				// when reqs is closed
				close(sess.winch)
			}()
			//req.Reply(ok, nil) // let the proxy reply
		case "window-change":
			if sess.pty == nil {
				req.Reply(false, nil)
				continue
			}
			win, ok := parseWinchRequest(req.Payload)
			if ok {
				sess.pty.Window = win
				sess.winch <- win
			}
			req.Reply(ok, nil)
		case "auth-agent-req@openssh.com":
			// TODO: option/callback to allow agent forwarding
			ssh.SetAgentRequested(sess.ctx)
			req.Reply(true, nil)
		default:
			// TODO: debug log
		}

		if addToMaskedReqs {
			sess.maskedReqs <- req
		}
	}
}
