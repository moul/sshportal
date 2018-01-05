package telnet


import (
	"crypto/tls"
)


func DialAndCall(caller Caller) error {
	conn, err := Dial()
	if nil != err {
		return err
	}

	client := &Client{Caller:caller}

	return client.Call(conn)
}


func DialToAndCall(srvAddr string, caller Caller) error {
	conn, err := DialTo(srvAddr)
	if nil != err {
		return err
	}

	client := &Client{Caller:caller}

	return client.Call(conn)
}


func DialAndCallTLS(caller Caller, tlsConfig *tls.Config) error {
	conn, err := DialTLS(tlsConfig)
	if nil != err {
		return err
	}

	client := &Client{Caller:caller}

	return client.Call(conn)
}

func DialToAndCallTLS(srvAddr string, caller Caller, tlsConfig *tls.Config) error {
	conn, err := DialToTLS(srvAddr, tlsConfig)
	if nil != err {
		return err
	}

	client := &Client{Caller:caller}

	return client.Call(conn)
}


type Client struct {
	Caller Caller

	Logger Logger
}


func (client *Client) Call(conn *Conn) error {

	logger := client.logger()


	caller := client.Caller
	if nil == caller {
		logger.Debug("Defaulted caller to StandardCaller.")
		caller = StandardCaller
	}


	var ctx Context = NewContext().InjectLogger(logger)

	var w Writer = conn
	var r Reader = conn

	caller.CallTELNET(ctx, w, r)
	conn.Close()


	return nil
}


func (client *Client) logger() Logger {
	logger := client.Logger
	if nil == logger {
		logger = internalDiscardLogger{}
	}

	return logger
}


func (client *Client) SetAuth(username string) {
//@TODO: #################################################
}
