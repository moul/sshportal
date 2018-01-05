package telnet


import (
	"crypto/tls"
	"net"
)


// ListenAndServeTLS acts identically to ListenAndServe, except that it
// uses the TELNET protocol over TLS.
//
// From a TELNET protocol point-of-view, it allows for 'secured telnet', also known as TELNETS,
// which by default listens to port 992.
//
// Of course, this port can be overridden using the 'addr' argument.
//
// For a very simple example:
//
//	package main
//	
//	import (
//		"github.com/reiver/go-telnet"
//	)
//	
//	func main() {
//		
//		//@TODO: In your code, you would probably want to use a different handler.
//		var handler telnet.Handler = telnet.EchoHandler
//		
//		err := telnet.ListenAndServeTLS(":5555", "cert.pem", "key.pem", handler)
//		if nil != err {
//			//@TODO: Handle this error better.
//			panic(err)
//		}
//	}
func ListenAndServeTLS(addr string, certFile string, keyFile string, handler Handler) error {
	server := &Server{Addr: addr, Handler: handler}
	return server.ListenAndServeTLS(certFile, keyFile)
}



// ListenAndServeTLS acts identically to ListenAndServe, except that it
// uses the TELNET protocol over TLS.
//
// From a TELNET protocol point-of-view, it allows for 'secured telnet', also known as TELNETS,
// which by default listens to port 992.
func (server *Server) ListenAndServeTLS(certFile string, keyFile string) error {

	addr := server.Addr
	if "" == addr {
		addr = ":telnets"
	}


	listener, err := net.Listen("tcp", addr)
	if nil != err {
		return err
	}


	// Apparently have to make a copy of the TLS config this way, rather than by
	// simple assignment, to prevent some unexported fields from being copied over.
	//
	// It would be nice if tls.Config had a method that would do this "safely".
	// (I.e., what happens if in the future more exported fields are added to
	// tls.Config?)
	var tlsConfig *tls.Config = nil
	if nil == server.TLSConfig {
		tlsConfig = &tls.Config{}
	} else {
		tlsConfig = &tls.Config{
			Rand:                     server.TLSConfig.Rand,
			Time:                     server.TLSConfig.Time,
			Certificates:             server.TLSConfig.Certificates,
			NameToCertificate:        server.TLSConfig.NameToCertificate,
			GetCertificate:           server.TLSConfig.GetCertificate,
			RootCAs:                  server.TLSConfig.RootCAs,
			NextProtos:               server.TLSConfig.NextProtos,
			ServerName:               server.TLSConfig.ServerName,
			ClientAuth:               server.TLSConfig.ClientAuth,
			ClientCAs:                server.TLSConfig.ClientCAs,
			InsecureSkipVerify:       server.TLSConfig.InsecureSkipVerify,
			CipherSuites:             server.TLSConfig.CipherSuites,
			PreferServerCipherSuites: server.TLSConfig.PreferServerCipherSuites,
			SessionTicketsDisabled:   server.TLSConfig.SessionTicketsDisabled,
			SessionTicketKey:         server.TLSConfig.SessionTicketKey,
			ClientSessionCache:       server.TLSConfig.ClientSessionCache,
			MinVersion:               server.TLSConfig.MinVersion,
			MaxVersion:               server.TLSConfig.MaxVersion,
			CurvePreferences:         server.TLSConfig.CurvePreferences,
		}
	}


	tlsConfigHasCertificate := len(tlsConfig.Certificates) > 0 || nil != tlsConfig.GetCertificate
	if "" == certFile || "" == keyFile || !tlsConfigHasCertificate {
		tlsConfig.Certificates = make([]tls.Certificate, 1)

		var err error
		tlsConfig.Certificates[0], err = tls.LoadX509KeyPair(certFile, keyFile)
		if nil != err {
			return err
		}
	}


	tlsListener := tls.NewListener(listener, tlsConfig)


	return server.Serve(tlsListener)
}
