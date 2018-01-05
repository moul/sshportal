package oi


import (
	"io"
)


// WriteNopCloser takes an io.Writer and returns an io.WriteCloser where
// calling the Write method on the returned io.WriterCloser calls the
// Write method on the io.Writer it received, but whre calling the Close
// method on the returned io.WriterCloser does "nothing" (i.e., is a "nop").
//
// This is useful in cases where an io.WriteCloser is expected, but you
// only have an io.Writer (where closing doesn't make sense) and you
// need to make your io.Writer fit. (I.e., you need an adaptor.)
func WriteNopCloser(w io.Writer) io.WriteCloser {
	wc := internalWriteNopCloser{
		writer:w,
	}

	return &wc
}


type internalWriteNopCloser struct {
	writer io.Writer
}


func (wc * internalWriteNopCloser) Write(p []byte) (n int, err error) {
	return wc.writer.Write(p)
}


func (wc * internalWriteNopCloser) Close() error {
	return nil
}
