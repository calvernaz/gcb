package gcb

import (
	"net/http"
)

// tripper
type tripper struct {
	http.RoundTripper
}

func NewRoundTripper(opts ...Option) *tripper {
	circuit := newCircuit(opts...)
	t := &tripper{
		RoundTripper: circuit,
	}
	return t
}

