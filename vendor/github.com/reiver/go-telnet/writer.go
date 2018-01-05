package telnet


type Writer interface {
	Write([]byte) (int, error)
}
