package oi


import (
	"io"
)


// LongWrite tries to write the bytes from 'p' to the writer 'w', such that it deals
// with "short writes" where w.Write would return an error of io.ErrShortWrite and
// n < len(p).
//
// Note that LongWrite still could return the error io.ErrShortWrite; but this
// would only be after trying to handle the io.ErrShortWrite a number of times, and
// then eventually giving up.
func LongWrite(w io.Writer, p []byte) (int64, error) {

	numWritten := int64(0)
	for {
//@TODO: Should check to make sure this doesn't get stuck in an infinite loop writting nothing!
		n, err := w.Write(p)
		numWritten += int64(n)
		if nil != err && io.ErrShortWrite != err {
			return numWritten, err
		}

		if !(n < len(p)) {
			break
		}

		p = p[n:]

		if len(p) < 1 {
			break
		}
	}

	return numWritten, nil
}
