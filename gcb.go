package gcb

import (
	"net/http"
)

// Gcb
type Gcb struct {
	http.RoundTripper
}

func New() *Gcb {
	circuit := newCircuit()
	gcb := &Gcb{
		RoundTripper: circuit,
	}
	return gcb
}
