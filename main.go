package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/gliderlabs/ssh"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/kr/pty"
	"github.com/urfave/cli"
	gossh "golang.org/x/crypto/ssh"
)

var (
	// Version should be updated by hand at each release
	Version = "1.7.1+dev"
	// GitTag will be overwritten automatically by the build system
	GitTag string
	// GitSha will be overwritten automatically by the build system
	GitSha string
	// GitBranch will be overwritten automatically by the build system
	GitBranch string
)

func main() {
	rand.Seed(time.Now().UnixNano())

	app := cli.NewApp()
	app.Name = path.Base(os.Args[0])
	app.Author = "Manfred Touron"
	app.Version = Version + " (" + GitSha + ")"
	app.Email = "https://github.com/moul/sshportal"
	app.Commands = []cli.Command{
		{
			Name:   "server",
			Usage:  "Start sshportal server",
			Action: server,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:   "bind-address, b",
					EnvVar: "SSHPORTAL_BIND",
					Value:  ":2222",
					Usage:  "SSH server bind address",
				},
				cli.StringFlag{
					Name:  "db-driver",
					Value: "sqlite3",
					Usage: "GORM driver (sqlite3)",
				},
				cli.StringFlag{
					Name:  "db-conn",
					Value: "./sshportal.db",
					Usage: "GORM connection string",
				},
				cli.BoolFlag{
					Name:  "debug, D",
					Usage: "Display debug information",
				},
				cli.StringFlag{
					Name:  "aes-key",
					Usage: "Encrypt sensitive data in database (length: 16, 24 or 32)",
				},
				cli.StringFlag{
					Name: "logs-location",
					Value: "./log",
					Usage: "Store user session files",
				},
			},
		}, {
			Name:   "healthcheck",
			Action: healthcheck,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "addr, a",
					Value: "localhost:2222",
					Usage: "sshportal server address",
				},
				cli.BoolFlag{
					Name:  "wait, w",
					Usage: "Loop indefinitely until sshportal is ready",
				},
				cli.BoolFlag{
					Name:  "quiet, q",
					Usage: "Do not print errors, if any",
				},
			},
		}, {
			Name:   "_test_server",
			Hidden: true,
			Action: testServer,
		},
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatalf("error: %v", err)
	}
}

func server(c *cli.Context) error {
	switch len(c.String("aes-key")) {
	case 0, 16, 24, 32:
	default:
		return fmt.Errorf("invalid aes key size, should be 16 or 24, 32")
	}
	// db
	db, err := gorm.Open(c.String("db-driver"), c.String("db-conn"))
	if err != nil {
		return err
	}
	defer func() {
		if err2 := db.Close(); err2 != nil {
			panic(err2)
		}
	}()
	if err = db.DB().Ping(); err != nil {
		return err
	}
	if c.Bool("debug") {
		db.LogMode(true)
	}
	if err = dbInit(db); err != nil {
		return err
	}

	// check for the logdir existence
	logsLocation, e := os.Stat(c.String("logs-location"))
	if e != nil {
		err = os.MkdirAll(c.String("logs-location"), os.ModeDir | os.FileMode(0750) )
		if err != nil {
			return err
		}
	} else {
		if !logsLocation.IsDir() {
			log.Fatal("log directory cannnot be created")
		}
	}
	
	opts := []ssh.Option{}
	// custom PublicKeyAuth handler
	opts = append(opts, ssh.PublicKeyAuth(publicKeyAuthHandler(db, c)))
	opts = append(opts, ssh.PasswordAuth(passwordAuthHandler(db, c)))

	// retrieve sshportal SSH private key from database
	opts = append(opts, func(srv *ssh.Server) error {
		var key SSHKey
		if err = SSHKeysByIdentifiers(db, []string{"host"}).First(&key).Error; err != nil {
			return err
		}
		SSHKeyDecrypt(c.String("aes-key"), &key)

		var signer gossh.Signer
		signer, err = gossh.ParsePrivateKey([]byte(key.PrivKey))
		if err != nil {
			return err
		}
		srv.AddHostKey(signer)
		return nil
	})

	// create TCP listening socket
	ln, err := net.Listen("tcp", c.String("bind-address"))
	if err != nil {
		return err
	}

	// configure server
	srv := &ssh.Server{
		Addr:           c.String("bind-address"),
		Handler:        shellHandler, // ssh.Server.Handler is the handler for the DefaultSessionHandler
		Version:        fmt.Sprintf("sshportal-%s", Version),
		ChannelHandler: channelHandler,
	}
	for _, opt := range opts {
		if err := srv.SetOption(opt); err != nil {
			return err
		}
	}

	log.Printf("info: SSH Server accepting connections on %s", c.String("bind-address"))
	return srv.Serve(ln)
}

// perform a healthcheck test without requiring an ssh client or an ssh key (used for Docker's HEALTHCHECK)
func healthcheck(c *cli.Context) error {
	config := gossh.ClientConfig{
		User:            "healthcheck",
		HostKeyCallback: func(hostname string, remote net.Addr, key gossh.PublicKey) error { return nil },
		Auth:            []gossh.AuthMethod{gossh.Password("healthcheck")},
	}

	if c.Bool("wait") {
		for {
			if err := healthcheckOnce(c.String("addr"), config, c.Bool("quiet")); err != nil {
				if !c.Bool("quiet") {
					log.Printf("error: %v", err)
				}
				time.Sleep(time.Second)
				continue
			}
			return nil
		}
	}

	if err := healthcheckOnce(c.String("addr"), config, c.Bool("quiet")); err != nil {
		if c.Bool("quiet") {
			return cli.NewExitError("", 1)
		}
		return err
	}
	return nil
}

func healthcheckOnce(addr string, config gossh.ClientConfig, quiet bool) error {
	client, err := gossh.Dial("tcp", addr, &config)
	if err != nil {
		return err
	}

	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer func() {
		if err := session.Close(); err != nil {
			if !quiet {
				log.Printf("failed to close session: %v", err)
			}
		}
	}()

	var b bytes.Buffer
	session.Stdout = &b
	if err := session.Run(""); err != nil {
		return err
	}
	stdout := strings.TrimSpace(b.String())
	if stdout != "OK" {
		return fmt.Errorf("invalid stdout: %q expected 'OK'", stdout)
	}
	return nil
}

// testServer is an hidden handler used for integration tests
func testServer(c *cli.Context) error {
	ssh.Handle(func(s ssh.Session) {
		helloMsg := struct {
			User    string
			Environ []string
			Command []string
		}{
			User:    s.User(),
			Environ: s.Environ(),
			Command: s.Command(),
		}
		enc := json.NewEncoder(s)
		if err := enc.Encode(&helloMsg); err != nil {
			log.Fatalf("failed to write helloMsg: %v", err)
		}
		var cmd *exec.Cmd
		if s.Command() == nil {
			cmd = exec.Command("/bin/sh") // #nosec
		} else {
			cmd = exec.Command(s.Command()[0], s.Command()[1:]...) // #nosec
		}
		ptyReq, winCh, isPty := s.Pty()
		var cmdErr error
		if isPty {
			cmd.Env = append(cmd.Env, fmt.Sprintf("TERM=%s", ptyReq.Term))
			f, err := pty.Start(cmd)
			if err != nil {
				fmt.Fprintf(s, "failed to run command: %v\n", err)
				_ = s.Exit(1)
				return
			}
			go func() {
				for win := range winCh {
					_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCSWINSZ),
						uintptr(unsafe.Pointer(&struct{ h, w, x, y uint16 }{uint16(win.Height), uint16(win.Width), 0, 0}))) // #nosec
				}
			}()
			go func() {
				_, _ = io.Copy(f, s) // stdin
			}()
			_, _ = io.Copy(s, f) // stdout
			cmdErr = cmd.Wait()
		} else {
			//cmd.Stdin = s
			cmd.Stdout = s
			cmd.Stderr = s
			cmdErr = cmd.Run()
		}

		if cmdErr != nil {
			if exitError, ok := cmdErr.(*exec.ExitError); ok {
				waitStatus := exitError.Sys().(syscall.WaitStatus)
				_ = s.Exit(waitStatus.ExitStatus())
				return
			}
		}
		waitStatus := cmd.ProcessState.Sys().(syscall.WaitStatus)
		_ = s.Exit(waitStatus.ExitStatus())
	})

	log.Println("starting ssh server on port 2222...")
	return ssh.ListenAndServe(":2222", nil)
}
