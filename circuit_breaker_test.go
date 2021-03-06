package gcb

import (
	"bufio"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/calvernaz/gcb/testutil"
)

func TestCircuit_FailedAllAttempts(t *testing.T) {
	client, _, _, teardown := newRoundTripper(WithMaxRetries(2))
	defer teardown()

	var i int
	for i < 2 {
		request, _ := http.NewRequest(http.MethodPost, "http://localhost", strings.NewReader("Hello Server!"))
		resp, err := client.Do(request)
		if err == nil {
			t.Fatal(err)
		}

		if resp != nil {
			if _, err = io.Copy(ioutil.Discard, resp.Body); err != nil {
				t.Error(err)
			}
		}
		i++
	}
}

func TestCircuit_DefaultRetryAttempts(t *testing.T) {
	// table tests
	tt := []struct {
		shouldRetry int
		statusCode  int
	}{
		{4, 200},
		{7, 500},
	}

	client, baseURL, mux, teardown := newRoundTripper()
	defer teardown()

	// setup mock handler
	var maxRetries int
	var reqNum int
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		reqNum++
		if reqNum <= maxRetries {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(200)
		w.Write([]byte("Hello Client!"))
	}))

	// tests
	for _, ts := range tt {
		maxRetries = ts.shouldRetry

		request, _ := http.NewRequest(http.MethodPost, baseURL, strings.NewReader("Hi Server!"))
		resp, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}

		if resp.StatusCode != ts.statusCode {
			t.Errorf("Expected %d, got %d", ts.statusCode, resp.StatusCode)
		}

		// reset request counter
		reqNum = 0

		if _, err = io.Copy(bufio.NewWriter(os.Stdout), resp.Body); err != nil {
			t.Error(err)
		}

	}
}

func TestCircuit_WithConfiguredRetryAttempts(t *testing.T) {
	// table tests
	tt := []struct {
		shouldRetry int
		statusCode  int
	}{
		{3, 500},
		{4, 500},
	}

	client, baseURL, mux, teardown := newRoundTripper(WithMaxRetries(2))
	defer teardown()

	// setup mock handler
	var maxRetries int
	var reqNum int
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		reqNum++
		if reqNum <= maxRetries {
			w.WriteHeader(500)
			return
		}
		w.WriteHeader(200)
	}))

	// tests
	for _, ts := range tt {
		maxRetries = ts.shouldRetry

		request, _ := http.NewRequest("GET", baseURL, nil)
		resp, err := client.Do(request)
		if err != nil {
			t.Error(err)
		}

		if resp.StatusCode != ts.statusCode {
			t.Errorf("Expected %d, got %d", ts.statusCode, resp.StatusCode)
		}

		// reset request counter
		reqNum = 0

		if _, err = io.Copy(ioutil.Discard, resp.Body); err != nil {
			t.Error(err)
		}

	}
}

func newRoundTripper(opts ...Option) (http.Client, string, *http.ServeMux, func()) {
	// setup http client with our round tripper
	// the default number of shouldRetry is 4.
	transport := NewRoundTripper(opts...)
	client := http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}
	// setup mock server
	baseURL, mux, teardown := testutil.ServerMock()
	return client, baseURL, mux, teardown
}
