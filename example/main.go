package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/calvernaz/gcb"
)

func main() {

	transport := gcb.New()
	client := http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	i := 0
	start := time.Now()
	for {
		request, _ := http.NewRequest("GET", "http://localhost:8080/", nil)
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
