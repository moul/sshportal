package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/jinzhu/gorm"
	"github.com/moul/sshportal/pkg/bastionsession"
	gossh "golang.org/x/crypto/ssh"
)

type sshportalContextKey string

var authContextKey = sshportalContextKey("auth")

type authContext struct {
	message       string
	err           error
	user          User
	inputUsername string
	db            *gorm.DB
	userKey       UserKey
	config        *configServe
	authMethod    string
	authSuccess   bool
}

type UserType string

const (
	UserTypeHealthcheck UserType = "healthcheck"
	UserTypeBastion              = "bastion"
	UserTypeInvite               = "invite"
	UserTypeShell                = "shell"
)

type SessionType string

const (
	SessionTypeBastion SessionType = "bastion"
	SessionTypeShell               = "shell"
)

func (c authContext) userType() UserType {
	switch {
	case c.inputUsername == "healthcheck":
		return UserTypeHealthcheck
	case c.inputUsername == c.user.Name || c.inputUsername == c.user.Email || c.inputUsername == "admin":
		return UserTypeShell
	case strings.HasPrefix(c.inputUsername, "invite:"):
		return UserTypeInvite
	default:
		return UserTypeBastion
	}
}

func (c authContext) sessionType() SessionType {
	switch c.userType() {
	case "bastion":
		return SessionTypeBastion
	default:
		return SessionTypeShell
	}
}

func dynamicHostKey(db *gorm.DB, host *Host) gossh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key gossh.PublicKey) error {
		if len(host.HostKey) == 0 {
			log.Println("Discovering host fingerprint...")
			return db.Model(host).Update("HostKey", key.Marshal()).Error
		}

		if !bytes.Equal(host.HostKey, key.Marshal()) {
			return fmt.Errorf("ssh: host key mismatch")
		}
		return nil
	}
}

func channelHandler(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
	switch newChan.ChannelType() {
	case "session":
	case "direct-tcpip":
	default:
		// TODO: handle direct-tcp (only for ssh scheme)
		if err := newChan.Reject(gossh.UnknownChannelType, "unsupported channel type"); err != nil {
			log.Printf("error: failed to reject channel: %v", err)
		}
		return
	}

	actx := ctx.Value(authContextKey).(*authContext)

	switch actx.userType() {
	case UserTypeBastion:
		log.Printf("New connection(bastion): sshUser=%q remote=%q local=%q dbUser=id:%q,email:%s", conn.User(), conn.RemoteAddr(), conn.LocalAddr(), actx.user.ID, actx.user.Email)
		host, err := HostByName(actx.db, actx.inputUsername)
		if err != nil {
			ch, _, err2 := newChan.Accept()
			if err2 != nil {
				return
			}
			fmt.Fprintf(ch, "error: %v\n", err)
			// FIXME: force close all channels
			_ = ch.Close()
			return
		}

		switch host.Scheme() {
		case BastionSchemeSSH:
			sessionConfigs := make([]bastionsession.Config, 0)
			currentHost := host
			for currentHost != nil {
				clientConfig, err2 := bastionClientConfig(ctx, currentHost)
				if err2 != nil {
					ch, _, err3 := newChan.Accept()
					if err3 != nil {
						return
					}
					fmt.Fprintf(ch, "error: %v\n", err2)
					// FIXME: force close all channels
					_ = ch.Close()
					return
				}
				sessionConfigs = append([]bastionsession.Config{{
					Addr:         currentHost.DialAddr(),
					ClientConfig: clientConfig,
					Logs:         actx.config.logsLocation,
				}}, sessionConfigs...)
				if currentHost.HopID != 0 {
					var newHost Host
					actx.db.Model(currentHost).Related(&newHost, "HopID")
					hostname := newHost.Name
					currentHost, _ = HostByName(actx.db, hostname)
				} else {
					currentHost = nil
				}
			}

			sess := Session{
				UserID: actx.user.ID,
				HostID: host.ID,
				Status: SessionStatusActive,
			}
			if err = actx.db.Create(&sess).Error; err != nil {
				ch, _, err2 := newChan.Accept()
				if err2 != nil {
					return
				}
				fmt.Fprintf(ch, "error: %v\n", err)
				_ = ch.Close()
				return
			}

			err = bastionsession.MultiChannelHandler(srv, conn, newChan, ctx, sessionConfigs)

			now := time.Now()
			sessUpdate := Session{
				Status:    SessionStatusClosed,
				ErrMsg:    fmt.Sprintf("%v", err),
				StoppedAt: &now,
			}
			switch sessUpdate.ErrMsg {
			case "lch closed the connection", "rch closed the connection":
				sessUpdate.ErrMsg = ""
			}
			actx.db.Model(&sess).Updates(&sessUpdate)
		case BastionSchemeTelnet:
			tmpSrv := ssh.Server{
				// PtyCallback: srv.PtyCallback,
				Handler: telnetHandler(host),
			}
			ssh.DefaultChannelHandler(&tmpSrv, conn, newChan, ctx)
		default:
			ch, _, err2 := newChan.Accept()
			if err2 != nil {
				return
			}
			fmt.Fprintf(ch, "error: unknown bastion scheme: %q\n", host.Scheme())
			// FIXME: force close all channels
			_ = ch.Close()
		}
	default: // shell
		ssh.DefaultChannelHandler(srv, conn, newChan, ctx)
	}
}

func bastionClientConfig(ctx ssh.Context, host *Host) (*gossh.ClientConfig, error) {
	actx := ctx.Value(authContextKey).(*authContext)

	clientConfig, err := host.clientConfig(dynamicHostKey(actx.db, host))
	if err != nil {
		return nil, err
	}

	var tmpUser User
	if err = actx.db.Preload("Groups").Preload("Groups.ACLs").Where("id = ?", actx.user.ID).First(&tmpUser).Error; err != nil {
		return nil, err
	}
	var tmpHost Host
	if err = actx.db.Preload("Groups").Preload("Groups.ACLs").Where("id = ?", host.ID).First(&tmpHost).Error; err != nil {
		return nil, err
	}
	action, err2 := CheckACLs(tmpUser, tmpHost)
	if err2 != nil {
		return nil, err2
	}

	HostDecrypt(actx.config.aesKey, host)
	SSHKeyDecrypt(actx.config.aesKey, host.SSHKey)

	switch action {
	case ACLActionAllow:
	case ACLActionDeny:
		return nil, fmt.Errorf("you don't have permission to that host")
	default:
		return nil, fmt.Errorf("invalid ACL action: %q", action)
	}
	return clientConfig, nil
}

func shellHandler(s ssh.Session) {
	actx := s.Context().Value(authContextKey).(*authContext)
	if actx.userType() != UserTypeHealthcheck {
		log.Printf("New connection(shell): sshUser=%q remote=%q local=%q command=%q dbUser=id:%q,email:%s", s.User(), s.RemoteAddr(), s.LocalAddr(), s.Command(), actx.user.ID, actx.user.Email)
	}

	if actx.err != nil {
		fmt.Fprintf(s, "error: %v\n", actx.err)
		_ = s.Exit(1)
		return
	}

	if actx.message != "" {
		fmt.Fprint(s, actx.message)
	}

	switch actx.userType() {
	case UserTypeHealthcheck:
		fmt.Fprintln(s, "OK")
		return
	case UserTypeShell:
		if err := shell(s); err != nil {
			fmt.Fprintf(s, "error: %v\n", err)
			_ = s.Exit(1)
		}
		return
	case UserTypeInvite:
		// do nothing (message was printed at the beginning of the function)
		return
	}
	panic("should not happen")
}

func passwordAuthHandler(db *gorm.DB, cfg *configServe) ssh.PasswordHandler {
	return func(ctx ssh.Context, pass string) bool {
		actx := &authContext{
			db:            db,
			inputUsername: ctx.User(),
			config:        cfg,
			authMethod:    "password",
		}
		actx.authSuccess = actx.userType() == UserTypeHealthcheck
		ctx.SetValue(authContextKey, actx)
		return actx.authSuccess
	}
}

func privateKeyFromDB(db *gorm.DB, aesKey string) func(*ssh.Server) error {
	return func(srv *ssh.Server) error {
		var key SSHKey
		if err := SSHKeysByIdentifiers(db, []string{"host"}).First(&key).Error; err != nil {
			return err
		}
		SSHKeyDecrypt(aesKey, &key)

		signer, err := gossh.ParsePrivateKey([]byte(key.PrivKey))
		if err != nil {
			return err
		}
		srv.AddHostKey(signer)
		return nil
	}
}

func publicKeyAuthHandler(db *gorm.DB, cfg *configServe) ssh.PublicKeyHandler {
	return func(ctx ssh.Context, key ssh.PublicKey) bool {
		actx := &authContext{
			db:            db,
			inputUsername: ctx.User(),
			config:        cfg,
			authMethod:    "pubkey",
			authSuccess:   true,
		}
		ctx.SetValue(authContextKey, actx)

		// lookup user by key
		db.Where("authorized_key = ?", string(gossh.MarshalAuthorizedKey(key))).First(&actx.userKey)
		if actx.userKey.UserID > 0 {
			db.Preload("Roles").Where("id = ?", actx.userKey.UserID).First(&actx.user)
			if actx.userType() == UserTypeInvite {
				actx.err = fmt.Errorf("invites are only supported for new SSH keys; your ssh key is already associated with the user %q", actx.user.Email)
			}
			return true
		}

		// handle invite "links"
		if actx.userType() == UserTypeInvite {
			inputToken := strings.Split(actx.inputUsername, ":")[1]
			if len(inputToken) > 0 {
				db.Where("invite_token = ?", inputToken).First(&actx.user)
			}
			if actx.user.ID > 0 {
				actx.userKey = UserKey{
					UserID:        actx.user.ID,
					Key:           key.Marshal(),
					Comment:       "created by sshportal",
					AuthorizedKey: string(gossh.MarshalAuthorizedKey(key)),
				}
				db.Create(&actx.userKey)

				// token is only usable once
				actx.user.InviteToken = ""
				db.Model(&actx.user).Updates(&actx.user)

				actx.message = fmt.Sprintf("Welcome %s!\n\nYour key is now associated with the user %q.\n", actx.user.Name, actx.user.Email)
			} else {
				actx.user = User{Name: "Anonymous"}
				actx.err = errors.New("your token is invalid or expired")
			}
			return true
		}

		// fallback
		actx.err = errors.New("unknown ssh key")
		actx.user = User{Name: "Anonymous"}
		return true
	}
}
