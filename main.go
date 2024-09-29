package main

import (
	"OpenSPMRegistry/controller"
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/", controller.MainAction)

	log.Fatal(http.ListenAndServe(":1234", nil))
}
