package main

import (
	"OpenSPMRegistry/controller"
	"flag"
	"log"
	"net/http"
)

var (
	tlsFlag bool
)

func main() {
	flag.BoolVar(&tlsFlag, "tls", false, "enable tls enabled")
	flag.Parse()

	http.HandleFunc("/", controller.MainAction)

	if tlsFlag {
		log.Println("Starting HTTPS server on :1234")
		log.Fatal(http.ListenAndServeTLS(":1234", "server.crt", "server.key", nil))
	} else {
		log.Println("Starting HTTP server on :1234")
		log.Fatal(http.ListenAndServe(":1234", nil))
	}
}
