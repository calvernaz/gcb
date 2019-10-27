package gcb

import (
	"net/http"
)

type State int8

const (
	Open State = iota + 1
	HalfOpen
	Close
)

func (s State) String() string {
	switch s {
	case Open:
		return "Open"
	case HalfOpen:
		return "HalfOpen"
	case Close:
		return "Close"
	}
	return ""
}

// Gcb
type Gcb struct {
	state State

	http.RoundTripper
}

func New() *Gcb {
	circuit := newCircuit()
	gcb := &Gcb{
		state:        Close, // starts as ready to work :)
		RoundTripper: circuit,
	}
	return gcb
}
