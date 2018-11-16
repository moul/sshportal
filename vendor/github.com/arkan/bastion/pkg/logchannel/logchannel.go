package logchannel

import (
	"encoding/binary"
	"io"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
)

type logChannel struct {
	channel ssh.Channel
	writer  io.WriteCloser
}

func writeTTYRecHeader(fd io.Writer, length int) {
	t := time.Now()

	tv := syscall.NsecToTimeval(t.UnixNano())

	binary.Write(fd, binary.LittleEndian, int32(tv.Sec))
	binary.Write(fd, binary.LittleEndian, int32(tv.Usec))
	binary.Write(fd, binary.LittleEndian, int32(length))
}

func New(channel ssh.Channel, writer io.WriteCloser) *logChannel {
	return &logChannel{
		channel: channel,
		writer:  writer,
	}
}

func (l *logChannel) Read(data []byte) (int, error) {
	return l.Read(data)
}

func (l *logChannel) Write(data []byte) (int, error) {
	writeTTYRecHeader(l.writer, len(data))
	l.writer.Write(data)

	return l.channel.Write(data)
}

func (l *logChannel) LogWrite(data []byte) (int, error) {
	writeTTYRecHeader(l.writer, len(data))
	return l.writer.Write(data)
}

func (l *logChannel) Close() error {
	l.writer.Close()

	return l.channel.Close()
}
