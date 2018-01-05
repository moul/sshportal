package oi


import (
	"io"
)


// LongWriteByte trys to write the byte from 'b' to the writer 'w', such that it deals
// with "short writes" where w.Write would return an error of io.ErrShortWrite and
// n < 1.
//
// Note that LongWriteByte still could return the error io.ErrShortWrite; but this
// would only be after trying to handle the io.ErrShortWrite a number of times, and
// then eventually giving up.
func LongWriteByte(w io.Writer, b byte) error {
	var buffer [1]byte
	p := buffer[:]

	buffer[0] = b

	numWritten, err := LongWrite(w, p)
	if 1 != numWritten {
		return io.ErrShortWrite
	}

	return err
}
