# go-telnet

Package **telnet** provides TELNET and TELNETS client and server implementations, for the Go programming language.


The **telnet** package provides an API in a style similar to the "net/http" library that is part of the Go standard library, including support for "middleware".


(TELNETS is *secure TELNET*, with the TELNET protocol over a secured TLS (or SSL) connection.)


## Documention

Online documentation, which includes examples, can be found at: http://godoc.org/github.com/reiver/go-telnet

[![GoDoc](https://godoc.org/github.com/reiver/go-telnet?status.svg)](https://godoc.org/github.com/reiver/go-telnet)


## Very Simple TELNET Server Example

A very very simple TELNET server is shown in the following code.

This particular TELNET server just echos back to the user anything they "submit" to the server.

(By default, a TELNET client does *not* send anything to the server until the [Enter] key is pressed.
"Submit" means typing something and then pressing the [Enter] key.)

```
package main

import (
	"github.com/reiver/go-telnet"
)

func main() {

	var handler telnet.Handler = telnet.EchoHandler
	
	err := telnet.ListenAndServe(":5555", handler)
	if nil != err {
		//@TODO: Handle this error better.
		panic(err)
	}
}

```

If you wanted to test out this very very simple TELNET server, if you were on the same computer it was
running, you could connect to it using the bash command:
```
telnet localhost 5555
```
(Note that we use the same TCP port number -- "5555" -- as we had in our code. That is important, as the
value used by your TELNET server and the value used by your TELNET client **must** match.)


## Very Simple (Secure) TELNETS Server Example

TELNETS is the secure version of TELNET.

The code to make a TELNETS server is very similar to the code to make a TELNET server. 
(The difference between we use the `telnet.ListenAndServeTLS` func instead of the
`telnet.ListenAndServe` func.)

```
package main

import (
	"github.com/reiver/go-telnet"
)

func main() {

	var handler telnet.Handler = telnet.EchoHandler
	
	err := telnet.ListenAndServeTLS(":5555", "cert.pem", "key.pem", handler)
	if nil != err {
		//@TODO: Handle this error better.
		panic(err)
	}
}

```

If you wanted to test out this very very simple TELNETS server, get the `telnets` client program from here:
https://github.com/reiver/telnets


## TELNET Client Example:
```
package main

import (
	"github.com/reiver/go-telnet"
)

func main() {
	var caller Caller = telnet.StandardCaller

	//@TOOD: replace "example.net:5555" with address you want to connect to.
	telnet.DialToAndCall("example.net:5555", caller)
}
```


## TELNETS Client Example:
```
package main

import (
	"github.com/reiver/go-telnet"

	"crypto/tls"
)

func main() {
	//@TODO: Configure the TLS connection here, if you need to.
	tlsConfig := &tls.Config{}

	var caller Caller = telnet.StandardCaller

	//@TOOD: replace "example.net:5555" with address you want to connect to.
	telnet.DialToAndCallTLS("example.net:5555", caller, tlsConfig)
}
```


##  TELNET Shell Server Example

A more useful TELNET servers can be made using the `"github.com/reiver/go-telnet/telsh"` sub-package.

For example:
```
package main


import (
	"github.com/reiver/go-oi"
	"github.com/reiver/go-telnet"
	"github.com/reiver/go-telnet/telsh"

	"fmt"
	"io"
	"time"
)


func fiveHandler(stdin io.ReadCloser, stdout io.WriteCloser, stderr io.WriteCloser)error {
	oi.LongWriteString(stdout, "The number FIVE looks like this: 5\r\n")

	return nil
}

func fiveProducer(ctx telsh.Context, name string, args ...string) telsh.Handler{
	return telsh.PromoteHandlerFunc(fiveHandler)
}



func danceHandler(stdin io.ReadCloser, stdout io.WriteCloser, stderr io.WriteCloser)error {
	for i:=0; i<20; i++ {
		oi.LongWriteString(stdout, "\r⠋")
		time.Sleep(50*time.Millisecond)

		oi.LongWriteString(stdout, "\r⠙")
		time.Sleep(50*time.Millisecond)

		oi.LongWriteString(stdout, "\r⠹")
		time.Sleep(50*time.Millisecond)

		oi.LongWriteString(stdout, "\r⠸")
		time.Sleep(50*time.Millisecond)

		oi.LongWriteString(stdout, "\r⠼")
		time.Sleep(50*time.Millisecond)

		oi.LongWriteString(stdout, "\r⠴")
		time.Sleep(50*time.Millisecond)

		oi.LongWriteString(stdout, "\r⠦")
		time.Sleep(50*time.Millisecond)

		oi.LongWriteString(stdout, "\r⠧")
		time.Sleep(50*time.Millisecond)

		oi.LongWriteString(stdout, "\r⠇")
		time.Sleep(50*time.Millisecond)

		oi.LongWriteString(stdout, "\r⠏")
		time.Sleep(50*time.Millisecond)
	}
	oi.LongWriteString(stdout, "\r \r\n")

	return nil
}

func danceProducer(ctx telsh.Context, name string, args ...string) telsh.Handler{

	return telsh.PromoteHandlerFunc(danceHandler)
}


func main() {

	shellHandler := telsh.NewShellHandler()

	shellHandler.WelcomeMessage = `
 __          __ ______  _        _____   ____   __  __  ______ 
 \ \        / /|  ____|| |      / ____| / __ \ |  \/  ||  ____|
  \ \  /\  / / | |__   | |     | |     | |  | || \  / || |__   
   \ \/  \/ /  |  __|  | |     | |     | |  | || |\/| ||  __|  
    \  /\  /   | |____ | |____ | |____ | |__| || |  | || |____ 
     \/  \/    |______||______| \_____| \____/ |_|  |_||______|

`


	// Register the "five" command.
	commandName     := "five"
	commandProducer := telsh.ProducerFunc(fiveProducer)

	shellHandler.Register(commandName, commandProducer)



	// Register the "dance" command.
	commandName      = "dance"
	commandProducer  = telsh.ProducerFunc(danceProducer)

	shellHandler.Register(commandName, commandProducer)



	shellHandler.Register("dance", telsh.ProducerFunc(danceProducer))

	addr := ":5555"
	if err := telnet.ListenAndServe(addr, shellHandler); nil != err {
		panic(err)
	}
}
```

TELNET servers made using the `"github.com/reiver/go-telnet/telsh"` sub-package will often be more useful
as it makes it easier for you to create a *shell* interface.


# More Information

There is a lot more information about documentation on all this here: http://godoc.org/github.com/reiver/go-telnet

(You should really read those.)

