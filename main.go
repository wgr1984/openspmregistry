package main

import (
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/controller"
	"OpenSPMRegistry/repo"
	"flag"
	"fmt"
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

	router := http.NewServeMux()

	repoConfig := serverConfig.Server.Repo

	if repoConfig.Type != "file" {
		log.Fatal("Only filesystem is supported as repo so far")
	}

	r := repo.NewFileRepo(repoConfig.Path)

	c := controller.NewController(serverConfig.Server, r)

	router.HandleFunc("GET /", c.MainAction)
	router.HandleFunc("GET /{scope}/{package}", c.ListAction)
	router.HandleFunc("GET /{scope}/{package}/{version}", c.InfoAction)
	router.HandleFunc("GET /{scope}/{package}/{version}/Package.swift", c.FetchManifestAction)
	router.HandleFunc("PUT /{scope}/{package}/{version}", c.PublishAction)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", serverConfig.Server.Port),
		Handler: http.Handler(router),
	}

	if tlsFlag {
		slog.Info("Starting HTTPS server on", "port", srv.Addr)
		log.Fatal(srv.ListenAndServeTLS("server.crt", "server.key"))
	} else {
		slog.Info("Starting HTTP server on", "port", srv.Addr)
		log.Fatal(srv.ListenAndServe())
	}
}
