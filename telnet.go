package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/gliderlabs/ssh"
	oi "github.com/reiver/go-oi"
	telnet "github.com/reiver/go-telnet"
)

type bastionTelnetCaller struct {
	ssh ssh.Session
}

func (caller bastionTelnetCaller) CallTELNET(ctx telnet.Context, w telnet.Writer, r telnet.Reader) {
	go func(writer io.Writer, reader io.Reader) {
		var buffer [1]byte // Seems like the length of the buffer needs to be small, otherwise will have to wait for buffer to fill up.
		p := buffer[:]

		for {
			// Read 1 byte.
			n, err := reader.Read(p)
			if n <= 0 && err == nil {
				continue
			} else if n <= 0 && err != nil {
				break
			}

			if _, err = oi.LongWrite(writer, p); err != nil {
				log.Printf("telnet longwrite failed: %v", err)
			}
		}
	}(caller.ssh, r)

	var buffer bytes.Buffer
	var p []byte

	var crlfBuffer = [2]byte{'\r', '\n'}
	crlf := crlfBuffer[:]

	scanner := bufio.NewScanner(caller.ssh)
	scanner.Split(scannerSplitFunc)

	for scanner.Scan() {
		buffer.Write(scanner.Bytes())
		buffer.Write(crlf)

		p = buffer.Bytes()

		n, err := oi.LongWrite(w, p)
		if nil != err {
			break
		}
		if expected, actual := int64(len(p)), n; expected != actual {
			err := fmt.Errorf("transmission problem: tried sending %d bytes, but actually only sent %d bytes", expected, actual)
			fmt.Fprint(caller.ssh, err.Error())
			return
		}
		buffer.Reset()
	}

	// Wait a bit to receive data from the server (that we would send to io.Stdout).
	time.Sleep(3 * time.Millisecond)
}

func scannerSplitFunc(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF {
		return 0, nil, nil
	}
	return bufio.ScanLines(data, atEOF)
}

func telnetHandler(host *Host) ssh.Handler {
	return func(s ssh.Session) {
		// FIXME: log session in db
		//actx := s.Context().Value(authContextKey).(*authContext)
		caller := bastionTelnetCaller{ssh: s}
		if err := telnet.DialToAndCall(host.DialAddr(), caller); err != nil {
			fmt.Fprintf(s, "error: %v", err)
		}
	}
}
