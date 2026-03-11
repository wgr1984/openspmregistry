package main

import (
	"OpenSPMRegistry/authenticator"
	"OpenSPMRegistry/config"
	"OpenSPMRegistry/controller"
	"OpenSPMRegistry/middleware"
	"OpenSPMRegistry/repo"
	"OpenSPMRegistry/repo/files"
	"OpenSPMRegistry/repo/maven"
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

// headResponseWriter discards the response body. Used for HEAD requests so the
// same GET handler runs (Go 1.22+ matches HEAD to GET) but no body is sent.
type headResponseWriter struct {
	http.ResponseWriter
}

var (
	verboseFlag bool
	configPath  string
)

func (h *headResponseWriter) Write(b []byte) (int, error) {
	return len(b), nil
}

func loadServerConfig() (*config.ServerRoot, error) {
	path := configPath
	if path == "" {
		if _, err := os.Stat("config.local.yml"); err == nil {
			path = "config.local.yml"
		} else {
			path = "config.yml"
		}
	}
	yamlData, err := os.ReadFile(path)
	if err != nil {
		return nil, err
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
	flag.BoolVar(&verboseFlag, "v", false, "show more information")
	flag.StringVar(&configPath, "config", "", "path to config file (default: config.local.yml or config.yml)")
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

	registryMux := http.NewServeMux()
	collectionMux := http.NewServeMux()

	repoConfig := serverConfig.Server.Repo

	var r repo.Repo
	switch repoConfig.Type {
	case "file":
		r = files.NewFileRepo(repoConfig.Path)
	case "maven":
		mavenRepo, err := maven.NewMavenRepo(repoConfig.Maven)
		if err != nil {
			log.Fatalf("Failed to create Maven repository: %v", err)
		}
		r = mavenRepo
	default:
		log.Fatalf("Unsupported repo type: %s", repoConfig.Type)
	}
	a := middleware.NewAuthentication(authenticator.CreateAuthenticator(serverConfig.Server), registryMux)
	c := controller.NewController(serverConfig.Server, r)

	// Package Collections on a separate mux so Go 1.22+ ServeMux does not conflict with /{scope}/{package}.
	// GET also matches HEAD per Go 1.22+ routing.
	allowAuthQueryParam := serverConfig.Server.PackageCollections.AllowAuthQueryParam
	if serverConfig.Server.PackageCollections.Enabled {
		if serverConfig.Server.PackageCollections.PublicRead {
			collectionMux.HandleFunc("GET /collection", c.GlobalCollectionAction)
			collectionMux.HandleFunc("GET /collection/{scope}", c.ScopeCollectionAction)
		} else {
			collectionMux.HandleFunc("GET /collection", a.WrapHandler(c.GlobalCollectionAction, allowAuthQueryParam))
			collectionMux.HandleFunc("GET /collection/{scope}", a.WrapHandler(c.ScopeCollectionAction, allowAuthQueryParam))
		}
	}

	// authorized routes (registry only). GET matches HEAD automatically.
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

	// public and static routes on registry mux
	registryMux.HandleFunc("GET /", c.MainAction)
	registryMux.HandleFunc("GET /favicon.ico", c.StaticAction)
	registryMux.HandleFunc("GET /favicon.svg", c.StaticAction)
	registryMux.HandleFunc("GET /output.css", c.StaticAction)

	// Path dispatcher: for HEAD, discard response body; then /collection* -> collectionMux, else -> auth-wrapped registryMux.
	// Go 1.22+ matches HEAD to GET patterns, so the same handler runs; we only strip the body.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w = &headResponseWriter{ResponseWriter: w}
		}
		if strings.HasPrefix(r.URL.Path, "/collection") {
			collectionMux.ServeHTTP(w, r)
		} else {
			a.ServeHTTP(w, r)
		}
	})

	addr := fmt.Sprintf(":%d", serverConfig.Server.Port)
	if serverConfig.Server.Hostname != "" {
		addr = fmt.Sprintf("%s:%d", serverConfig.Server.Hostname, serverConfig.Server.Port)
	}
	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}

	sigChannel := make(chan os.Signal, 1)
	signal.Notify(sigChannel, os.Interrupt, syscall.SIGTERM)

	const shutdownTimeout = 30 * time.Second
	go func() {
		<-sigChannel
		slog.Info("Shutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			slog.Error("Error shutting down server", "error", err)
		}
		os.Exit(1)
	}()

	if serverConfig.Server.TlsEnabled {
		slog.Info("Starting HTTPS server on", "port", srv.Addr)
		certFile := serverConfig.Server.Certs.CertFile
		keyFile := serverConfig.Server.Certs.KeyFile
		log.Fatal(srv.ListenAndServeTLS(certFile, keyFile))
	} else {
		slog.Info("Starting HTTP server on", "port", srv.Addr)
		log.Fatal(srv.ListenAndServe())
	}
}
