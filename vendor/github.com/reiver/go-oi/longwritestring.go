package oi


import (
	"io"
)


// LongWriteString tries to write the bytes from 's' to the writer 'w', such that it deals
// with "short writes" where w.Write (or w.WriteString) would return an error of io.ErrShortWrite
// and n < len(s).
//
// Note that LongWriteString still could return the error io.ErrShortWrite; but this
// would only be after trying to handle the io.ErrShortWrite a number of times, and
// then eventually giving up.
func LongWriteString(w io.Writer, s string) (int64, error) {

	numWritten := int64(0)
	for {
//@TODO: Should check to make sure this doesn't get stuck in an infinite loop writting nothing!
		n, err := io.WriteString(w, s)
		numWritten += int64(n)
		if nil != err && io.ErrShortWrite != err {
			return numWritten, err
		}

		if !(n < len(s)) {
			break
		}

		s = s[n:]

		if len(s) < 1 {
			break
		}
	}

	return numWritten, nil
}
