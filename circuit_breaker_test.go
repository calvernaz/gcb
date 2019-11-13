package gcb

import (
	"github.com/calvernaz/gcb/testutil"
	"io"
	"log"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestCircuitRetry(t *testing.T) {
	baseURL, mux, teardown := testutil.ServerMock()
	defer teardown()

	var reqNum int
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		reqNum++
		if reqNum < 4 {
			w.WriteHeader(500)
			return
		}

		w.WriteHeader(200)
		_, _ = w.Write([]byte("Hello, world!"))

	}))

	//c := ExternalServiceClient{BaseURL: baseURL}
	//result, err := c.Call()
	//// ...
	//
	//if expectedReqNum := 1; reqNum != expectedReqNum {
	//	t.Errorf("ExternalServiceClient.Call() expected to make %d request(s), but it sent %d instead", expectedReqNum, reqNum)
	//}
	//


	//circuit := &tripper{
	//	RoundTripper: http.DefaultTransport,
	//}

	transport := NewRoundTripper()
	client := http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	request, _ := http.NewRequest("GET", baseURL, nil)
	resp, err := client.Do(request)
	if err != nil {
		log.Fatal(err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}
	_, _ = io.Copy(os.Stdout, resp.Body)
}
