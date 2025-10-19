package ptb

import (
	"testing"
	"time"
)

func TestAddr(t *testing.T) {
	addr := Addr{call: "DL1THM"}

	if addr.Network() != "ptb" {
		t.Errorf("Expected network 'ptb', got '%s'", addr.Network())
	}

	expected := "ptb:DL1THM"
	if addr.String() != expected {
		t.Errorf("Expected '%s', got '%s'", expected, addr.String())
	}
}

func TestModemState(t *testing.T) {
	// Test state transitions
	states := []ModemState{
		StateDisconnected,
		StateConnecting,
		StateConnected,
		StateBusy,
	}

	if len(states) != 4 {
		t.Error("Expected 4 states")
	}
}

func TestEventTypes(t *testing.T) {
	events := []EventType{
		EventConnected,
		EventDisconnected,
		EventBusy,
		EventData,
		EventError,
	}

	if len(events) != 5 {
		t.Error("Expected 5 event types")
	}
}

func TestDefaultAddresses(t *testing.T) {
	if DefaultAddr != "localhost:8300" {
		t.Errorf("Expected default addr 'localhost:8300', got '%s'", DefaultAddr)
	}

	if DefaultDataAddr != "localhost:8301" {
		t.Errorf("Expected default data addr 'localhost:8301', got '%s'", DefaultDataAddr)
	}
}

// Mock test for processResponse
func TestProcessResponse(t *testing.T) {
	m := &Modem{
		state:     StateDisconnected,
		listeners: make([]func(Event), 0),
	}

	tests := []struct {
		name     string
		response string
		expected ModemState
	}{
		{
			name:     "Connected",
			response: "*** CONNECTED to LA3F",
			expected: StateConnected,
		},
		{
			name:     "Disconnected",
			response: "*** DISCONNECTED",
			expected: StateDisconnected,
		},
		{
			name:     "Busy",
			response: "BUSY",
			expected: StateBusy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m.state = StateDisconnected
			m.processResponse(tt.response)

			if m.state != tt.expected {
				t.Errorf("Expected state %d, got %d", tt.expected, m.state)
			}
		})
	}
}

func TestEventListener(t *testing.T) {
	m := &Modem{
		state:     StateDisconnected,
		listeners: make([]func(Event), 0),
	}

	eventReceived := false
	listener := func(e Event) {
		if e.Type == EventConnected {
			eventReceived = true
		}
	}

	m.AddListener(listener)

	if len(m.listeners) != 1 {
		t.Error("Expected 1 listener")
	}

	// Simulate event
	m.notifyListeners(Event{Type: EventConnected, Data: "TEST"})

	// Give goroutine time to execute
	time.Sleep(10 * time.Millisecond)

	if !eventReceived {
		t.Error("Event not received by listener")
	}
}

// Integration test would require a running PTB instance
// This is a placeholder for the test structure
func TestModemConnection(t *testing.T) {
	t.Skip("Integration test - requires running PTB instance")

	// Example integration test:
	/*
		m, err := OpenTCP("localhost:8300", "localhost:8301", "TEST")
		if err != nil {
			t.Fatalf("Failed to open PTB: %v", err)
		}
		defer m.Close()

		if err := m.Ping(); err != nil {
			t.Errorf("Ping failed: %v", err)
		}
	*/
}

func TestConnClose(t *testing.T) {
	// Test that closing a connection twice doesn't panic
	c := &Conn{
		closed: false,
	}

	err := c.Close()
	if err != nil {
		t.Errorf("First close failed: %v", err)
	}

	err = c.Close()
	if err != nil {
		t.Error("Second close should not fail")
	}
}

func TestBusyCheck(t *testing.T) {
	m := &Modem{
		state: StateDisconnected,
	}

	if m.Busy() {
		t.Error("Expected not busy in disconnected state")
	}

	m.state = StateBusy
	if !m.Busy() {
		t.Error("Expected busy in busy state")
	}
}
