package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/calvernaz/gcb"
)

func main()  {

	circuit := gcb.NewCircuit()
	client := http.Client{
		Transport:     circuit,
		Timeout:       30 * time.Second,
	}

	request, _ := http.NewRequest("GET", "http://localhost:8080", nil)
	response, err := client.Do(request)
	if err != nil {
		log.Fatal(err)
	}
	bytes, err := ioutil.ReadAll(response.Body)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(bytes))
}
