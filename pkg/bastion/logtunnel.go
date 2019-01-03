package bastion // import "moul.io/sshportal/pkg/bastion"

import (
	"encoding/binary"
	"errors"
	"io"
	"log"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
)

type logTunnel struct {
	host    string
	channel ssh.Channel
	writer  io.WriteCloser
}

type logTunnelForwardData struct {
	DestinationHost string
	DestinationPort uint32
	SourceHost      string
	SourcePort      uint32
}

func writeHeader(fd io.Writer, length int) {
	t := time.Now()

	tv := syscall.NsecToTimeval(t.UnixNano())

	if err := binary.Write(fd, binary.LittleEndian, int32(tv.Sec)); err != nil {
		log.Printf("failed to write log header: %v", err)
	}
	if err := binary.Write(fd, binary.LittleEndian, tv.Usec); err != nil {
		log.Printf("failed to write log header: %v", err)
	}
	if err := binary.Write(fd, binary.LittleEndian, int32(length)); err != nil {
		log.Printf("failed to write log header: %v", err)
	}
}

func newLogTunnel(channel ssh.Channel, writer io.WriteCloser, host string) io.ReadWriteCloser {
	return &logTunnel{
		host:    host,
		channel: channel,
		writer:  writer,
	}
}

func (l *logTunnel) Read(data []byte) (int, error) {
	return 0, errors.New("logTunnel.Read is not implemented")
}

func (l *logTunnel) Write(data []byte) (int, error) {
	writeHeader(l.writer, len(data)+len(l.host+": "))
	if _, err := l.writer.Write([]byte(l.host + ": ")); err != nil {
		log.Printf("failed to write log: %v", err)
	}
	if _, err := l.writer.Write(data); err != nil {
		log.Printf("failed to write log: %v", err)
	}

	return l.channel.Write(data)
}

func (l *logTunnel) Close() error {
	l.writer.Close()

	return l.channel.Close()
}
