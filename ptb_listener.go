package ptb

import (
	"errors"
	"log"
	"net"
	"sync"
)

// Listener implements net.Listener for PTB
type Listener struct {
	modem   *Modem
	connCh  chan net.Conn
	closeCh chan struct{}
	once    sync.Once
	err     error
}

// NewListener creates a new PTB listener
func (m *Modem) NewListener() (*Listener, error) {
	l := &Listener{
		modem:   m,
		connCh:  make(chan net.Conn, 1),
		closeCh: make(chan struct{}),
	}

	// Register event listener for incoming connections
	listener := func(e Event) {
		if e.Type == EventConnected {
			conn := &Conn{
				modem:      m,
				remotecall: e.Data,
				localcall:  m.mycall,
			}

			// Store as active connection in modem
			m.mu.Lock()
			m.activeConn = conn
			m.mu.Unlock()

			// Setup disconnect listener for this connection
			conn.setupDisconnectListener()

			select {
			case l.connCh <- conn:
			case <-l.closeCh:
				// Listener closed, reject connection
				err := conn.Close()
				if err != nil {
					log.Printf("ptb: failed to close listener: %v", err)
				}
			}
		}
	}

	m.AddListener(listener)

	// Start listen mode
	if err := m.Listen(); err != nil {
		return nil, err
	}

	return l, nil
}

// Accept waits for and returns the next connection
func (l *Listener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.connCh:
		return conn, nil
	case <-l.closeCh:
		return nil, l.err
	}
}

// Close closes the listener
func (l *Listener) Close() error {
	l.once.Do(func() {
		l.err = errors.New("listener closed")
		close(l.closeCh)
	})
	return nil
}

// Addr returns the listener's network address
func (l *Listener) Addr() net.Addr {
	return Addr{call: l.modem.mycall}
}

// Implement net.Listener interface
var _ net.Listener = (*Listener)(nil)
