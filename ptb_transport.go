// Package ptb provides a transport for Pat using the PACTOR-TCP-Bridge (PTB)
// PTB connects to SCS PACTOR modems via WA8DED Hostmode over TCP
package ptb

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/la5nta/wl2k-go/transport"
)

const (
	DefaultAddr     = "localhost:8300"
	DefaultDataAddr = "localhost:8301"
)

// Modem represents a PTB (PACTOR-TCP-Bridge) connection
type Modem struct {
	addr     string
	dataAddr string
	mycall   string

	cmdConn  net.Conn
	dataConn net.Conn

	cmdReader *bufio.Reader
	mu        sync.Mutex

	listeners []func(Event)
	eventsMu  sync.RWMutex

	state      ModemState
	remotecall string
	bandwidth  int

	debug bool
}

// ModemState represents the current state of the modem
type ModemState int

const (
	StateDisconnected ModemState = iota
	StateConnecting
	StateConnected
	StateBusy
)

// Event represents events from the modem
type Event struct {
	Type EventType
	Data string
}

type EventType int

const (
	EventConnected EventType = iota
	EventDisconnected
	EventBusy
	EventData
	EventError
)

// OpenTCP opens a connection to PTB
func OpenTCP(addr, dataAddr, mycall string) (*Modem, error) {
	if addr == "" {
		addr = DefaultAddr
	}
	if dataAddr == "" {
		// If data address not specified, use command port + 1
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("invalid address format: %w", err)
		}
		var cmdPort int
		fmt.Sscanf(port, "%d", &cmdPort)
		dataAddr = fmt.Sprintf("%s:%d", host, cmdPort+1)
	}

	m := &Modem{
		addr:      addr,
		dataAddr:  dataAddr,
		mycall:    mycall,
		listeners: make([]func(Event), 0),
		state:     StateDisconnected,
		debug:     true,
	}

	// Connect to command socket
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to PTB command socket: %w", err)
	}
	m.cmdConn = conn
	m.cmdReader = bufio.NewReader(conn)

	// Connect to data socket
	dataConn, err := net.DialTimeout("tcp", dataAddr, 10*time.Second)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to connect to PTB data socket: %w", err)
	}
	m.dataConn = dataConn

	// Initialize modem
	if err := m.initialize(); err != nil {
		m.Close()
		return nil, fmt.Errorf("failed to initialize PTB: %w", err)
	}

	// Start event handler
	go m.handleEvents()

	return m, nil
}

// initialize sends initial commands to PTB
func (m *Modem) initialize() error {

	// Set callsign
	if err := m.sendCommand(fmt.Sprintf("I %s", m.mycall)); err != nil {
		return err
	}

	return nil
}

// sendCommand sends a command to the command socket
func (m *Modem) sendCommand(cmd string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.debug {
		log.Printf("PTB TX CMD: %s", cmd)
	}

	_, err := fmt.Fprintf(m.cmdConn, "%s\r", cmd)
	return err
}

// handleEvents processes incoming events from PTB
func (m *Modem) handleEvents() {
	for {
		line, err := m.cmdReader.ReadString('\r')
		if err != nil {
			if m.debug {
				log.Printf("PTB RX error: %v", err)
			}
			m.notifyListeners(Event{Type: EventError, Data: err.Error()})
			return
		}

		line = strings.TrimSpace(line)
		if m.debug {
			log.Printf("PTB RX CMD: %s", line)
		}

		m.processResponse(line)
	}
}

// processResponse interprets WA8DED hostmode responses
func (m *Modem) processResponse(resp string) {
	if len(resp) == 0 {
		return
	}

	switch {
	case strings.Contains(resp, "CONNECTED to"):
		m.state = StateConnected
		parts := strings.Fields(resp)
		if len(parts) >= 4 {
			m.remotecall = parts[3]
		}
		m.notifyListeners(Event{Type: EventConnected, Data: m.remotecall})

	case strings.Contains(resp, "DISCONNECTED fm"):
		m.state = StateDisconnected
		m.remotecall = ""
		m.notifyListeners(Event{Type: EventDisconnected})

	case strings.Contains(resp, "CHANNEL NOT CONNECTED"):
		m.state = StateDisconnected
		m.remotecall = ""
		m.notifyListeners(Event{Type: EventDisconnected})

	case strings.Contains(resp, "BUSY"):
		m.state = StateBusy
		m.notifyListeners(Event{Type: EventBusy})

	case strings.Contains(resp, "LINK FAILURE"):
		m.state = StateDisconnected
		m.remotecall = ""
		m.notifyListeners(Event{Type: EventError, Data: "Link failure"})

	default:
		// Other responses
		if m.debug {
			log.Printf("PTB: Unhandled response: %s", resp)
		}
	}
}

// Dial establishes a connection to a remote station
func (m *Modem) Dial(targetcall string) (*Conn, error) {
	m.mu.Lock()
	if m.state != StateDisconnected {
		m.mu.Unlock()
		return nil, errors.New("modem not in disconnected state")
	}
	m.state = StateConnecting
	m.mu.Unlock()

	// Send connect command
	cmd := fmt.Sprintf("C %s", targetcall)
	if err := m.sendCommand(cmd); err != nil {
		m.state = StateDisconnected
		return nil, err
	}

	// Wait for connection or timeout
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	connected := make(chan bool, 1)
	listener := func(e Event) {
		if e.Type == EventConnected {
			connected <- true
		} else if e.Type == EventError || e.Type == EventDisconnected {
			connected <- false
		}
	}

	m.AddListener(listener)
	defer m.RemoveListener(listener)

	select {
	case success := <-connected:
		if !success {
			return nil, errors.New("connection failed")
		}
	case <-ctx.Done():
		m.state = StateDisconnected
		return nil, errors.New("connection timeout")
	}

	return &Conn{
		modem:      m,
		remotecall: targetcall,
		localcall:  m.mycall,
	}, nil
}

// Listen puts the modem in listen mode
func (m *Modem) Listen() error {
	return m.sendCommand("%L1")
}

// Busy returns whether the channel is busy
func (m *Modem) Busy() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state == StateBusy
}

// AddListener registers an event listener
func (m *Modem) AddListener(fn func(Event)) {
	m.eventsMu.Lock()
	defer m.eventsMu.Unlock()
	m.listeners = append(m.listeners, fn)
}

// RemoveListener removes an event listener
func (m *Modem) RemoveListener(fn func(Event)) {
	m.eventsMu.Lock()
	defer m.eventsMu.Unlock()
	for i, l := range m.listeners {
		// Compare function pointers - this is a simple approach
		if &l == &fn {
			m.listeners = append(m.listeners[:i], m.listeners[i+1:]...)
			return
		}
	}
}

// notifyListeners sends an event to all registered listeners
func (m *Modem) notifyListeners(e Event) {
	m.eventsMu.RLock()
	defer m.eventsMu.RUnlock()
	for _, listener := range m.listeners {
		go listener(e)
	}
}

// SetDebug enables/disables debug output
func (m *Modem) SetDebug(debug bool) {
	m.debug = debug
}

// Close closes the PTB connection
func (m *Modem) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error

	if m.cmdConn != nil {
		// Send disconnect if connected
		if m.state == StateConnected {
			fmt.Fprintf(m.cmdConn, "D\r")
			time.Sleep(100 * time.Millisecond)
		}
		if err := m.cmdConn.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if m.dataConn != nil {
		if err := m.dataConn.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing PTB: %v", errs)
	}

	return nil
}

// Ping verifies the connection is still alive
func (m *Modem) Ping() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cmdConn == nil {
		return errors.New("not connected")
	}

	// Set a short deadline for the ping
	m.cmdConn.SetDeadline(time.Now().Add(2 * time.Second))
	defer m.cmdConn.SetDeadline(time.Time{})

	// Send status request
	_, err := fmt.Fprintf(m.cmdConn, "@B\r")
	return err
}

// Implement transport.Dialer interface
var _ transport.Dialer = (*Modem)(nil)

func (m *Modem) DialURL(url *transport.URL) (net.Conn, error) {
	return m.Dial(url.Target)
}
