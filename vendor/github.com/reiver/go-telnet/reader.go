package telnet


type Reader interface {
	Read([]byte) (int, error)
}
