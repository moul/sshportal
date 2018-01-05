/*
Package oi provides useful tools to be used with Go's standard "io" package.

For example, did you know that when you call the Write method on something that fits the io.Writer
interface, that it is possible that not everything was be written?!

I.e., that a 'short write' happened.

That just doing the following is (in general) not enough:

	n, err := writer.Write(p)

That, for example, you should be checking if "err == io.ErrShortWrite", and then maybe calling the Write
method again but only with what didn't get written.

For a simple example of this (that actually is not sufficient to solve this problem, but illustrates
the direction you would need to go to solve this problem is):

	n, err := w.Write(p)
	
	if io.ErrShortWrite == err {
		n2, err2 := w.Write(p[n:])
	}

Note that the second call to the Write method passed "p[n:]" (instead of just "p"), to account for the "n" bytes
already being written (with the first call to the `Write` method).

A more "production quality" version of this would likely be in a loop, but such that that the loop had "guards"
against looping forever, and also possibly looping for "too long".


Well package oi provides tools that helps you deal with this and other problems. For example, you
can handle a 'short write' with the following oi func:
```
n, err := oi.LongWrite(writer, p)
```

*/
package oi
