package ptb

import "fmt"

// Addr implements net.Addr for PTB connections
type Addr struct {
	call string
}

// Network returns the network type
func (a Addr) Network() string {
	return "ptb"
}

// String returns the callsign
func (a Addr) String() string {
	return fmt.Sprintf("ptb:%s", a.call)
}
