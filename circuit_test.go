package gcb

import (
	"net/http"
	"testing"
	"time"
)

func TestStateChanges(t *testing.T) {
	states := [] State {
		Open,
		HalfOpen,
		Close,
	}

	circuit := circuit{
		state:   0,
	}
	for i, s := range states {
		circuit.state = s
		if circuit.state != states[i] {
			t.Errorf("expected %s, got %s", states[i], circuit.state)
		}
	}
}


func TestCircuitRetry(t *testing.T) {

	retrier := NewRetrier()
	circuit := &circuit{
		retrier: retrier,

		RoundTripper: http.DefaultTransport,

	}

	client := http.Client{
		Transport:     circuit,
		Timeout:       30 * time.Second,
	}

	request, _ := http.NewRequest("GET", "/", nil)
	_, _ = client.Do(request)
}
