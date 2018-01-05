package telnet


// A Handler serves a TELNET (or TELNETS) connection.
//
// Writing data to the Writer passed as an argument to the ServeTELNET method
// will send data to the TELNET (or TELNETS) client.
//
// Reading data from the Reader passed as an argument to the ServeTELNET method
// will receive data from the TELNET client.
//
// The Writer's Write method sends "escaped" TELNET (and TELNETS) data.
//
// The Reader's Read method "un-escapes" TELNET (and TELNETS) data, and filters
// out TELNET (and TELNETS) command sequences.
type Handler interface {
	ServeTELNET(Context, Writer, Reader)
}
