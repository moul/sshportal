/*
Package telnet provides TELNET and TELNETS client and server implementations
in a style similar to the "net/http" library that is part of the Go standard library,
including support for "middleware"; TELNETS is secure TELNET, with the TELNET protocol
over a secured TLS (or SSL) connection.


Example TELNET Server

ListenAndServe starts a (un-secure) TELNET server with a given address and handler.

	handler := telnet.EchoHandler
	
	err := telnet.ListenAndServe(":23", handler)
	if nil != err {
		panic(err)
	}


Example TELNETS Server

ListenAndServeTLS starts a (secure) TELNETS server with a given address and handler,
using the specified "cert.pem" and "key.pem" files.

	handler := telnet.EchoHandler
	
	err := telnet.ListenAndServeTLS(":992", "cert.pem", "key.pem", handler)
	if nil != err {
		panic(err)
	}


Example TELNET Client:

DialToAndCall creates a (un-secure) TELNET client, which connects to a given address using the specified caller.

	package main
	
	import (
		"github.com/reiver/go-telnet"
	)
	
	func main() {
		var caller Caller = telnet.StandardCaller

		//@TOOD: replace "example.net:23" with address you want to connect to.
		telnet.DialToAndCall("example.net:23", caller)
	}


Example TELNETS Client:

DialToAndCallTLS creates a (secure) TELNETS client, which connects to a given address using the specified caller.

	package main
	
	import (
		"github.com/reiver/go-telnet"

		"crypto/tls"
	)
	
	func main() {
		//@TODO: Configure the TLS connection here, if you need to.
		tlsConfig := &tls.Config{}

		var caller Caller = telnet.StandardCaller
		
		//@TOOD: replace "example.net:992" with address you want to connect to.
		telnet.DialToAndCallTLS("example.net:992", caller, tlsConfig)
	}


TELNET vs TELNETS

If you are communicating over the open Internet, you should be using (the secure) TELNETS protocol and ListenAndServeTLS.

If you are communicating just on localhost, then using just (the un-secure) TELNET protocol and telnet.ListenAndServe may be OK.

If you are not sure which to use, use TELNETS and ListenAndServeTLS.


Example TELNET Shell Server

The previous 2 exaple servers were very very simple. Specifically, they just echoed back whatever
you submitted to it.

If you typed:

	Apple Banana Cherry\r\n

... it would send back:

	Apple Banana Cherry\r\n

(Exactly the same data you sent it.)

A more useful TELNET server can be made using the "github.com/reiver/go-telnet/telsh" sub-package.

The `telsh` sub-package provides "middleware" that enables you to create a "shell" interface (also
called a "command line interface" or "CLI") which most people would expect when using TELNET OR TELNETS.

For example:


	package main
	
	
	import (
		"github.com/reiver/go-oi"
		"github.com/reiver/go-telnet"
		"github.com/reiver/go-telnet/telsh"
		
		"time"
	)
	
	
	func main() {
		
		shellHandler := telsh.NewShellHandler()
		
		commandName := "date"
		shellHandler.Register(commandName, danceProducer)
		
		commandName = "animate"
		shellHandler.Register(commandName, animateProducer)
		
		addr := ":23"
		if err := telnet.ListenAndServe(addr, shellHandler); nil != err {
			panic(err)
		}
	}

Note that in the example, so far, we have registered 2 commands: `date` and `animate`.

For this to actually work, we need to have code for the `date` and `animate` commands.

The actual implemenation for the `date` command could be done like the following:

	func dateHandlerFunc(stdin io.ReadCloser, stdout io.WriteCloser, stderr io.WriteCloser) error {
		const layout = "Mon Jan 2 15:04:05 -0700 MST 2006"
		s := time.Now().Format(layout)
		
		if _, err := oi.LongWriteString(stdout, s); nil != err {
			return err
		}
		
		return nil
	}
	
	
	func dateProducerFunc(ctx telsh.Context, name string, args ...string) telsh.Handler{
		return telsh.PromoteHandlerFunc(dateHandler)
	}
	
	
	var dateProducer = ProducerFunc(dateProducerFunc)

Note that your "real" work is in the `dateHandlerFunc` func.

And the actual implementation for the `animate` command could be done as follows:

	func animateHandlerFunc(stdin io.ReadCloser, stdout io.WriteCloser, stderr io.WriteCloser) error {
		
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
	
	
	func animateProducerFunc(ctx telsh.Context, name string, args ...string) telsh.Handler{
		return telsh.PromoteHandlerFunc(animateHandler)
	}
	
	
	var animateProducer = ProducerFunc(animateProducerFunc)

Again, note that your "real" work is in the `animateHandlerFunc` func.

Generating PEM Files

If you are using the telnet.ListenAndServeTLS func or the telnet.Server.ListenAndServeTLS method, you will need to
supply "cert.pem" and "key.pem" files.

If you do not already have these files, the Go soure code contains a tool for generating these files for you.

It can be found at:

	$GOROOT/src/crypto/tls/generate_cert.go

So, for example, if your `$GOROOT` is the "/usr/local/go" directory, then it would be at:

	/usr/local/go/src/crypto/tls/generate_cert.go

If you run the command:

	go run $GOROOT/src/crypto/tls/generate_cert.go --help

... then you get the help information for "generate_cert.go".

Of course, you would replace or set `$GOROOT` with whatever your path actually is. Again, for example,
if your `$GOROOT` is the "/usr/local/go" directory, then it would be:

	go run /usr/local/go/src/crypto/tls/generate_cert.go --help

To demonstrate the usage of "generate_cert.go", you might run the following to generate certificates
that were bound to the hosts `127.0.0.1` and `localhost`:

	go run /usr/local/go/src/crypto/tls/generate_cert.go --ca --host='127.0.0.1,localhost'


If you are not sure where "generate_cert.go" is on your computer, on Linux and Unix based systems, you might
be able to find the file with the command:

	locate /src/crypto/tls/generate_cert.go

(If it finds it, it should output the full path to this file.)


Example TELNET Client

You can make a simple (un-secure) TELNET client with code like the following:

	package main
	
	
	import (
		"github.com/reiver/go-telnet"
	)
	
	
	func main() {
		var caller Caller = telnet.StandardCaller
		
		//@TOOD: replace "example.net:5555" with address you want to connect to.
		telnet.DialToAndCall("example.net:5555", caller)
	}


Example TELNETS Client

You can make a simple (secure) TELNETS client with code like the following:

	package main
	
	
	import (
		"github.com/reiver/go-telnet"
	)
	
	
	func main() {
		var caller Caller = telnet.StandardCaller
		
		//@TOOD: replace "example.net:5555" with address you want to connect to.
		telnet.DialToAndCallTLS("example.net:5555", caller)
	}


TELNET Story

The TELNET protocol is best known for providing a means of connecting to a remote computer, using a (text-based) shell interface, and being able to interact with it, (more or less) as if you were sitting at that computer.

(Shells are also known as command-line interfaces or CLIs.)

Although this was the original usage of the TELNET protocol, it can be (and is) used for other purposes as well.


The Era

The TELNET protocol came from an era in computing when text-based shell interface where the common way of interacting with computers.

The common interface for computers during this era was a keyboard and a monochromatic (i.e., single color) text-based monitors called "video terminals".

(The word "video" in that era of computing did not refer to things such as movies. But instead was meant to contrast it with paper. In particular, the teletype machines, which were typewriter like devices that had a keyboard, but instead of having a monitor had paper that was printed onto.)


Early Office Computers

In that era, in the early days of office computers, it was rare that an individual would have a computer at their desk. (A single computer was much too expensive.)

Instead, there would be a single central computer that everyone would share. The style of computer used (for the single central shared computer) was called a "mainframe".

What individuals would have at their desks, instead of their own compuer, would be some type of video terminal.

The different types of video terminals had named such as:

• VT52

• VT100

• VT220

• VT240

("VT" in those named was short for "video terminal".)


Teletype

To understand this era, we need to go back a bit in time to what came before it: teletypes.


Terminal Codes

Terminal codes (also sometimes called 'terminal control codes') are used to issue various kinds of commands
to the terminal.

(Note that 'terminal control codes' are a completely separate concept for 'TELNET commands',
and the two should NOT be conflated or confused.)

The most common types of 'terminal codes' are the 'ANSI escape codes'. (Although there are other types too.)


ANSI Escape Codes

ANSI escape codes (also sometimes called 'ANSI escape sequences') are a common type of 'terminal code' used
to do things such as:

• moving the cursor,

• erasing the display,

• erasing the line,

• setting the graphics mode,

• setting the foregroup color,

• setting the background color,

• setting the screen resolution, and

• setting keyboard strings.


Setting The Foreground Color With ANSI Escape Codes

One of the abilities of ANSI escape codes is to set the foreground color.

Here is a table showing codes for this:

	| ANSI Color   | Go string  | Go []byte                     |
	| ------------ | ---------- | ----------------------------- |
	| Black        | "\x1b[30m" | []byte{27, '[', '3','0', 'm'} |
	| Red          | "\x1b[31m" | []byte{27, '[', '3','1', 'm'} |
	| Green        | "\x1b[32m" | []byte{27, '[', '3','2', 'm'} |
	| Brown/Yellow | "\x1b[33m" | []byte{27, '[', '3','3', 'm'} |
	| Blue         | "\x1b[34m" | []byte{27, '[', '3','4', 'm'} |
	| Magenta      | "\x1b[35m" | []byte{27, '[', '3','5', 'm'} |
	| Cyan         | "\x1b[36m" | []byte{27, '[', '3','6', 'm'} |
	| Gray/White   | "\x1b[37m" | []byte{27, '[', '3','7', 'm'} |

(Note that in the `[]byte` that the first `byte` is the number `27` (which
is the "escape" character) where the third and fouth characters are the
**not** number literals, but instead character literals `'3'` and whatever.)


Setting The Background Color With ANSI Escape Codes

Another of the abilities of ANSI escape codes is to set the background color.

	| ANSI Color   | Go string  | Go []byte                     |
	| ------------ | ---------- | ----------------------------- |
	| Black        | "\x1b[40m" | []byte{27, '[', '4','0', 'm'} |
	| Red          | "\x1b[41m" | []byte{27, '[', '4','1', 'm'} |
	| Green        | "\x1b[42m" | []byte{27, '[', '4','2', 'm'} |
	| Brown/Yellow | "\x1b[43m" | []byte{27, '[', '4','3', 'm'} |
	| Blue         | "\x1b[44m" | []byte{27, '[', '4','4', 'm'} |
	| Magenta      | "\x1b[45m" | []byte{27, '[', '4','5', 'm'} |
	| Cyan         | "\x1b[46m" | []byte{27, '[', '4','6', 'm'} |
	| Gray/White   | "\x1b[47m" | []byte{27, '[', '4','7', 'm'} |

(Note that in the `[]byte` that the first `byte` is the number `27` (which
is the "escape" character) where the third and fouth characters are the
**not** number literals, but instead character literals `'4'` and whatever.)

Using ANSI Escape Codes

In Go code, if I wanted to use an ANSI escape code to use a blue background,
a white foreground, and bold, I could do that with the ANSI escape code:

	"\x1b[44;37;1m"

Note that that start with byte value 27, which we have encoded as hexadecimal
as \x1b. Followed by the '[' character.

Coming after that is the sub-string "44", which is the code that sets our background color to blue.

We follow that with the ';' character (which separates codes).

And the after that comes the sub-string "37", which is the code that set our foreground color to white.

After that, we follow with another ";" character (which, again, separates codes).

And then we follow it the sub-string "1", which is the code that makes things bold.

And finally, the ANSI escape sequence is finished off with the 'm' character.

To show this in a more complete example, our `dateHandlerFunc` from before could incorporate ANSI escape sequences as follows:

	func dateHandlerFunc(stdin io.ReadCloser, stdout io.WriteCloser, stderr io.WriteCloser) error {
		const layout = "Mon Jan 2 15:04:05 -0700 MST 2006"
		s := "\x1b[44;37;1m" + time.Now().Format(layout) + "\x1b[0m"
		
		if _, err := oi.LongWriteString(stdout, s); nil != err {
			return err
		}
		
		return nil
	}

Note that in that example, in addition to using the ANSI escape sequence "\x1b[44;37;1m"
to set the background color to blue, set the foreground color to white, and make it bold,
we also used the ANSI escape sequence "\x1b[0m" to reset the background and foreground colors
and boldness back to "normal".

*/
package telnet
