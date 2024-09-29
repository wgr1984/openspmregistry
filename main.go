package main

import (
	"OpenSPMRegistry/controller"
	"flag"
	"github.com/gorilla/mux"
	"log"
	"log/slog"
	"net/http"
)

var (
	tlsFlag     bool
	verboseFlag bool
)

func main() {
	flag.BoolVar(&tlsFlag, "tls", false, "enable tls enabled")
	flag.BoolVar(&verboseFlag, "v", false, "show more information")
	flag.Parse()

	if verboseFlag {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	} else {
		slog.SetLogLoggerLevel(slog.LevelInfo)
	}

	router := mux.NewRouter()

	router.HandleFunc("/", controller.MainAction)
	router.HandleFunc("/{scope}/{package}/{version}", controller.PublishAction).Methods("PUT")

	srv := &http.Server{
		Addr:    ":1234",
		Handler: http.Handler(router),
	}

	if tlsFlag {
		slog.Info("Starting HTTPS server on", srv.Addr)
		log.Fatal(srv.ListenAndServeTLS("server.crt", "server.key"))
	} else {
		slog.Info("Starting HTTP server on", srv.Addr)
		log.Fatal(srv.ListenAndServe())
	}
}
