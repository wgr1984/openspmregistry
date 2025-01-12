package main

import (
	"OpenSPMRegistry/authenticator"
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/controller"
	"OpenSPMRegistry/middleware"
	"OpenSPMRegistry/repo/files"
	"flag"
	"fmt"
	"gopkg.in/yaml.v3"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

var (
	tlsFlag     bool
	verboseFlag bool
)

func loadServerConfig() (*config.ServerRoot, error) {
	yamlData, err := os.ReadFile("config.local.yml")
	if err != nil {
		yamlData, err = os.ReadFile("config.yml")
		if err != nil {
			return nil, err
		}
	}

	serverRoot := &config.ServerRoot{
		Server: config.ServerConfig{
			Auth: config.AuthConfig{
				Enabled: true, // enable authentication by default
			},
		},
	}
	if err := yaml.Unmarshal(yamlData, &serverRoot); err != nil {
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

	r := files.NewFileRepo(repoConfig.Path)
	a := middleware.NewAuthentication(authenticator.CreateAuthenticator(serverConfig.Server), router)
	c := controller.NewController(serverConfig.Server, r)

	// authorized routes
	a.HandleFunc("POST /login", c.LoginAction)
	a.HandleFunc("GET /{scope}/{package}", c.ListAction)
	a.HandleFunc("GET /{scope}/{package}/{version}", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".zip") {
			c.DownloadSourceArchiveAction(w, r)
		} else {
			c.InfoAction(w, r)
		}
	})
	a.HandleFunc("GET /{scope}/{package}/{version}/Package.swift", c.FetchManifestAction)
	a.HandleFunc("GET /identifiers", c.LookupAction)
	a.HandleFunc("PUT /{scope}/{package}/{version}", c.PublishAction)

	// public routes
	router.HandleFunc("GET /", c.MainAction)

	// static routes
	router.HandleFunc("GET /favicon.ico", c.StaticAction)
	router.HandleFunc("GET /favicon.svg", c.StaticAction)
	router.HandleFunc("GET /output.css", c.StaticAction)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", serverConfig.Server.Port),
		Handler: a,
	}

	sigChannel := make(chan os.Signal, 1)
	signal.Notify(sigChannel, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChannel
		slog.Info("Shutting down server...")
		if err := srv.Shutdown(nil); err != nil {
			slog.Error("Error shutting down server", "error", err)
		}
		os.Exit(1)
	}()

	if tlsFlag {
		slog.Info("Starting HTTPS server on", "port", srv.Addr)
		certFile := serverConfig.Server.Certs.CertFile
		keyFile := serverConfig.Server.Certs.KeyFile
		log.Fatal(srv.ListenAndServeTLS(certFile, keyFile))
	} else {
		slog.Info("Starting HTTP server on", "port", srv.Addr)
		log.Fatal(srv.ListenAndServe())
	}
}
