package telnet


// A Caller represents the client end of a TELNET (or TELNETS) connection.
//
// Writing data to the Writer passed as an argument to the CallTELNET method
// will send data to the TELNET (or TELNETS) server.
//
// Reading data from the Reader passed as an argument to the CallTELNET method
// will receive data from the TELNET server.
//
// The Writer's Write method sends "escaped" TELNET (and TELNETS) data.
//
// The Reader's Read method "un-escapes" TELNET (and TELNETS) data, and filters
// out TELNET (and TELNETS) command sequences.
type Caller interface {
	CallTELNET(Context, Writer, Reader)
}
