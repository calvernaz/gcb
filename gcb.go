package gcb

import (
	"net/http"
)

// tripper
type tripper struct {
	http.RoundTripper
}

func NewRoundTripper() *tripper {
	circuit := newCircuit()
	gcb := &tripper{
		RoundTripper: circuit,
	}
	return gcb
}
