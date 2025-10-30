package ptb

import (
	"io"
	"log"
	"net"
	"sync"
	"time"
)

// Conn represents an established connection through PTB
type Conn struct {
	modem              *Modem
	remotecall         string
	localcall          string
	closed             bool
	mu                 sync.Mutex
	disconnectListener func(Event)
}

// Read reads data from the data socket
func (c *Conn) Read(p []byte) (n int, err error) {
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()

	if closed {
		return 0, io.EOF
	}

	n, err = c.modem.dataConn.Read(p)

	// If we got an error and the connection was closed during the read,
	// return EOF instead of the deadline error
	c.mu.Lock()
	if c.closed && err != nil {
		c.mu.Unlock()
		return n, io.EOF
	}
	c.mu.Unlock()

	return n, err
}

// Write writes data to the data socket
func (c *Conn) Write(p []byte) (n int, err error) {
	if c.closed {
		err := c.Close()
		if err != nil {
			log.Printf("ptb: failed to close conn: %v", err)
		}
		return 0, io.ErrClosedPipe
	}

	return c.modem.dataConn.Write(p)
}

// Close disconnects and closes the connection
func (c *Conn) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	listener := c.disconnectListener
	c.mu.Unlock()

	// Remove the disconnect listener
	if listener != nil {
		c.modem.RemoveListener(listener)
	}

	// Disconnect from remote
	c.modem.Disconnect()

	// Interrupt any blocking Read operations
	c.modem.interruptRead()

	// Clear the active connection reference in modem
	c.modem.mu.Lock()
	if c.modem.activeConn == c {
		c.modem.activeConn = nil
	}
	c.modem.mu.Unlock()

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

// setupDisconnectListener registers a listener to handle remote disconnects
func (c *Conn) setupDisconnectListener() {
	// Clear any existing deadline from previous connection to ensure clean state
	c.modem.clearReadDeadline()

	listener := func(e Event) {
		if e.Type == EventDisconnected || e.Type == EventError {
			c.mu.Lock()
			if !c.closed {
				c.closed = true
				c.mu.Unlock()

				// Remove the listener to prevent it from being called again
				c.modem.RemoveListener(c.disconnectListener)

				// Interrupt any blocking Read operations
				c.modem.interruptRead()

				// Clear the active connection reference in modem
				c.modem.mu.Lock()
				if c.modem.activeConn == c {
					c.modem.activeConn = nil
				}
				c.modem.mu.Unlock()
			} else {
				c.mu.Unlock()
			}
		}
	}

	c.disconnectListener = listener
	c.modem.AddListener(listener)
}

// Implement net.Conn interface
var _ net.Conn = (*Conn)(nil)
