package main

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
)

func TestHealthcheckResponse(t *testing.T) {
	for _, response := range []string{"OK", "unexpected response"} {
		t.Run(response, func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer ln.Close()

			srv := &ssh.Server{
				Handler: func(s ssh.Session) {
					_, _ = fmt.Fprintln(s, response)
				},
				PasswordHandler: func(ctx ssh.Context, password string) bool {
					return ctx.User() == "healthcheck" && password == "healthcheck"
				},
			}
			defer srv.Close()
			go func() { _ = srv.Serve(ln) }()

			err = healthcheck(ln.Addr().String(), false, false)
			if response == "OK" {
				if err != nil {
					t.Fatalf("healthcheck() error = %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "invalid stdout") {
				t.Fatalf("healthcheck() error = %v; want invalid stdout", err)
			}
		})
	}
}

func TestHealthcheckTimesOutDuringSSHHandshake(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	release := make(chan struct{})
	accepted := make(chan struct{})
	defer close(release)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		close(accepted)
		<-release
	}()

	cfg := gossh.ClientConfig{
		User:            "healthcheck",
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
	}
	result := make(chan error, 1)
	go func() {
		result <- healthcheckOnce(ln.Addr().String(), cfg, 500*time.Millisecond)
	}()

	select {
	case err := <-result:
		select {
		case <-accepted:
		default:
			t.Fatal("server never accepted the connection")
		}
		if err == nil || !strings.Contains(err.Error(), "i/o timeout") {
			t.Fatalf("healthcheckOnce() error = %v; want a timeout", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("healthcheck stalled during the SSH handshake")
	}
}
