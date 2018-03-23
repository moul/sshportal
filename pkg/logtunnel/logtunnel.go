package logtunnel

import (
	"encoding/binary"
	"io"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
)

type logTunnel struct {
	host    string
	channel ssh.Channel
	writer  io.WriteCloser
}

type ForwardData struct {
	DestinationHost string
	DestinationPort uint32
	SourceHost string
	SourcePort uint32
}

func writeHeader(fd io.Writer, length int) {
	t := time.Now()

	tv := syscall.NsecToTimeval(t.UnixNano())

	binary.Write(fd, binary.LittleEndian, int32(tv.Sec))
	binary.Write(fd, binary.LittleEndian, int32(tv.Usec))
	binary.Write(fd, binary.LittleEndian, int32(length))
}

func New(channel ssh.Channel, writer io.WriteCloser, host string) *logTunnel {
	return &logTunnel{
		host: host,
		channel: channel,
		writer:  writer,
	}
}

func (l *logTunnel) Read(data []byte) (int, error) {
	return l.Read(data)
}

func (l *logTunnel) Write(data []byte) (int, error) {
	writeHeader(l.writer, len(data) + len(l.host + ": "))
	l.writer.Write([]byte(l.host + ": "))
	l.writer.Write(data)

	return l.channel.Write(data)
}

func (l *logTunnel) Close() error {
	l.writer.Close()

	return l.channel.Close()
}
