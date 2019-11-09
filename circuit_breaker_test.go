package gcb

import (
	"net/http"
	"testing"
	"time"
)

//func TestStateChanges(t *testing.T) {
//	states := []State{
//		Open,
//		HalfOpen,
//		Close,
//	}
//
//	circuit := Gcb{
//		state: 0,
//	}
//	for i, s := range states {
//		circuit.state = s
//		if circuit.state != states[i] {
//			t.Errorf("expected %s, got %s", states[i], circuit.state)
//		}
//	}
//}

func TestCircuitRetry(t *testing.T) {

	circuit := &Gcb{
		RoundTripper: http.DefaultTransport,
	}
	client := http.Client{
		Transport: circuit,
		Timeout:   30 * time.Second,
	}

	request, _ := http.NewRequest("GET", "/", nil)
	_, _ = client.Do(request)
}
