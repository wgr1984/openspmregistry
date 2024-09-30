package main

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/controller"
	"flag"
	"github.com/gorilla/mux"
	"gopkg.in/yaml.v3"
	"log"
	"log/slog"
	"net/http"
	"os"
)

var (
	tlsFlag      bool
	verboseFlag  bool
	serverConfig config.ServerRoot
)

func loadServerConfig() (*config.ServerRoot, error) {
	yamlData, err := os.ReadFile("config.yml")
	if err != nil {
		return nil, err
	}
	var serverRoot *config.ServerRoot
	err = yaml.Unmarshal(yamlData, &serverRoot)
	if err != nil {
		return nil, err
	}
	return serverRoot, nil
}

func main() {
	flag.BoolVar(&tlsFlag, "tls", false, "enable tls enabled")
	flag.BoolVar(&verboseFlag, "v", false, "show more information")
	flag.Parse()

	if verboseFlag {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	} else {
		slog.SetLogLoggerLevel(slog.LevelInfo)
	}

	serverConfig, err := loadServerConfig()
	if err != nil {
		log.Fatal(err)
	}

	router := mux.NewRouter()

	c := controller.NewController(serverConfig.Server)

	router.HandleFunc("/", c.MainAction)
	router.HandleFunc("/{scope}/{package}/{version}", c.PublishAction).Methods("PUT")

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
