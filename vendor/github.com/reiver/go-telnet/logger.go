package telnet


type Logger interface{
	Debug(...interface{})
	Debugf(string, ...interface{})

	Error(...interface{})
	Errorf(string, ...interface{})

	Trace(...interface{})
	Tracef(string, ...interface{})

	Warn(...interface{})
	Warnf(string, ...interface{})
}
