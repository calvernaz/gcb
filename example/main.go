package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/calvernaz/gcb"
)

func main() {

	transport := gcb.NewRoundTripper()
	client := http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	i := 0
	start := time.Now()
	for {
		request, _ := http.NewRequest(http.MethodPost, "http://localhost:8080/", strings.NewReader("Hello Server!"))
		response, err := client.Do(request)
		if err != nil {
			break
			//log.Fatal(err)
		}
		i++
		bytes, err := ioutil.ReadAll(response.Body)
		if err != nil {
			break
			//log.Fatal(err)
		}
		fmt.Println(string(bytes))
		fmt.Println(i)
	}
	t := time.Now()
	fmt.Println(t.Sub(start))
}
