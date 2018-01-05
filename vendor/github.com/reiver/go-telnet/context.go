package telnet


type Context interface {
	Logger() Logger

	InjectLogger(Logger) Context
}


type internalContext struct {
	logger Logger
}


func NewContext() Context {
	ctx := internalContext{}

	return &ctx
}


func (ctx *internalContext) Logger() Logger {
	return ctx.logger
}

func (ctx *internalContext) InjectLogger(logger Logger) Context {
	ctx.logger = logger

	return ctx
}
