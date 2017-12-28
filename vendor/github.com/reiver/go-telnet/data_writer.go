package telnet


import (
	"github.com/reiver/go-oi"

	"bytes"
	"errors"
	"io"
)


var iaciac []byte = []byte{255, 255}

var errOverflow = errors.New("Overflow")
var errPartialIACIACWrite = errors.New("Partial IAC IAC write.")


// An internalDataWriter deals with "escaping" according to the TELNET (and TELNETS) protocol.
//
// In the TELNET (and TELNETS) protocol byte value 255 is special.
//
// The TELNET (and TELNETS) protocol calls byte value 255: "IAC". Which is short for "interpret as command".
//
// The TELNET (and TELNETS) protocol also has a distinction between 'data' and 'commands'.
//
//(DataWriter is targetted toward TELNET (and TELNETS) 'data', not TELNET (and TELNETS) 'commands'.)
//
// If a byte with value 255 (=IAC) appears in the data, then it must be escaped.
//
// Escaping byte value 255 (=IAC) in the data is done by putting 2 of them in a row.
//
// So, for example:
//
//	[]byte{255} -> []byte{255, 255}
//
// Or, for a more complete example, if we started with the following:
//
//	[]byte{1, 55, 2, 155, 3, 255, 4, 40, 255, 30, 20}
//
// ... TELNET escaping would produce the following:
//
//	[]byte{1, 55, 2, 155, 3, 255, 255, 4, 40, 255, 255, 30, 20}
//
// (Notice that each "255" in the original byte array became 2 "255"s in a row.)
//
// internalDataWriter takes care of all this for you, so you do not have to do it.
type internalDataWriter struct {
	wrapped io.Writer
}


// newDataWriter creates a new internalDataWriter writing to 'w'.
//
// 'w' receives what is written to the *internalDataWriter but escaped according to
// the TELNET (and TELNETS) protocol.
//
// I.e., byte 255 (= IAC) gets encoded as 255, 255.
//
// For example, if the following it written to the *internalDataWriter's Write method:
//
//	[]byte{1, 55, 2, 155, 3, 255, 4, 40, 255, 30, 20}
//
// ... then (conceptually) the following is written to 'w's Write method:
//
//	[]byte{1, 55, 2, 155, 3, 255, 255, 4, 40, 255, 255, 30, 20}
//
// (Notice that each "255" in the original byte array became 2 "255"s in a row.)
//
// *internalDataWriter takes care of all this for you, so you do not have to do it.
func newDataWriter(w io.Writer) *internalDataWriter {
	writer := internalDataWriter{
		wrapped:w,
	}

	return &writer
}


// Write writes the TELNET (and TELNETS) escaped data for of the data in 'data' to the wrapped io.Writer.
func (w *internalDataWriter) Write(data []byte) (n int, err error) {
	var n64 int64

	n64, err = w.write64(data)
	n = int(n64)
	if int64(n) != n64 {
		panic(errOverflow)
	}

	return n, err
}


func (w *internalDataWriter) write64(data []byte) (n int64, err error) {

	if len(data) <= 0 {
		return 0, nil
	}

	const IAC = 255

	var buffer bytes.Buffer
	for _, datum := range data {

		if IAC == datum {

			if buffer.Len() > 0 {
				var numWritten int64

				numWritten, err = oi.LongWrite(w.wrapped, buffer.Bytes())
				n += numWritten
				if nil != err {
					return n, err
				}
				buffer.Reset()
			}


			var numWritten int64
			//@TODO: Should we worry about "iaciac" potentially being modified by the .Write()?
			numWritten, err = oi.LongWrite(w.wrapped, iaciac)
			if int64(len(iaciac)) != numWritten {
				//@TODO: Do we really want to panic() here?
				panic(errPartialIACIACWrite)
			}
			n += 1
			if nil != err {
				return n, err
			}
		} else {
			buffer.WriteByte(datum) // The returned error is always nil, so we ignore it.
		}
	}

	if buffer.Len() > 0 {
		var numWritten int64
		numWritten, err = oi.LongWrite(w.wrapped, buffer.Bytes())
		n += numWritten
	}

	return n, err
}
