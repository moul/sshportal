package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
	"syscall"
	"unsafe"

	"github.com/gliderlabs/ssh"
	"github.com/kr/pty"
	"github.com/urfave/cli"
)

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

		if err := json.NewEncoder(s).Encode(&helloMsg); err != nil {
			log.Fatalf("failed to write helloMsg: %v", err)
		}
		cmd := exec.Command(s.Command()[0], s.Command()[1:]...) // #nosec
		if s.Command() == nil {
			cmd = exec.Command("/bin/sh") // #nosec
		}
		ptyReq, winCh, isPty := s.Pty()
		var cmdErr error
		if isPty {
			cmd.Env = append(cmd.Env, fmt.Sprintf("TERM=%s", ptyReq.Term))
			f, err := pty.Start(cmd)
			if err != nil {
				fmt.Fprintf(s, "failed to run command: %v\n", err) // #nosec
				_ = s.Exit(1)                                      // #nosec
				return
			}
			go func() {
				for win := range winCh {
					_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCSWINSZ),
						uintptr(unsafe.Pointer(&struct{ h, w, x, y uint16 }{uint16(win.Height), uint16(win.Width), 0, 0}))) // #nosec
				}
			}()
			go func() {
				// stdin
				_, _ = io.Copy(f, s) // #nosec
			}()
			// stdout
			_, _ = io.Copy(s, f) // #nosec
			cmdErr = cmd.Wait()
		} else {
			//cmd.Stdin = s
			cmd.Stdout = s
			cmd.Stderr = s
			cmdErr = cmd.Run()
		}

		if cmdErr != nil {
			if exitError, ok := cmdErr.(*exec.ExitError); ok {
				_ = s.Exit(exitError.Sys().(syscall.WaitStatus).ExitStatus()) // #nosec
				return
			}
		}
		_ = s.Exit(cmd.ProcessState.Sys().(syscall.WaitStatus).ExitStatus()) // #nosec
	})

	log.Println("starting ssh server on port 2222...")
	return ssh.ListenAndServe(":2222", nil)
}
