package ptb

import (
	"io"
	"net"
	"time"
)

// Conn represents an established connection through PTB
type Conn struct {
	modem      *Modem
	remotecall string
	localcall  string
	closed     bool
}

// Read reads data from the data socket
func (c *Conn) Read(p []byte) (n int, err error) {
	if c.closed {
		return 0, io.EOF
	}

	return c.modem.dataConn.Read(p)
}

// Write writes data to the data socket
func (c *Conn) Write(p []byte) (n int, err error) {
	if c.closed {
		return 0, io.ErrClosedPipe
	}

	return c.modem.dataConn.Write(p)
}

// Close disconnects and closes the connection
func (c *Conn) Close() error {
	if c.closed {
		return nil
	}
	c.modem.Disconnect()
	c.closed = true
	return nil
}

// LocalAddr returns the local address
func (c *Conn) LocalAddr() net.Addr {
	return Addr{call: c.localcall}
}

// RemoteAddr returns the remote address
func (c *Conn) RemoteAddr() net.Addr {
	return Addr{call: c.remotecall}
}

// SetDeadline sets read and write deadlines
func (c *Conn) SetDeadline(t time.Time) error {
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}
	return c.SetWriteDeadline(t)
}

// SetReadDeadline sets the read deadline
func (c *Conn) SetReadDeadline(t time.Time) error {
	return c.modem.dataConn.SetReadDeadline(t)
}

// SetWriteDeadline sets the write deadline
func (c *Conn) SetWriteDeadline(t time.Time) error {
	return c.modem.dataConn.SetWriteDeadline(t)
}

// Implement net.Conn interface
var _ net.Conn = (*Conn)(nil)
