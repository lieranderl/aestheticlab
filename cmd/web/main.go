// Command web is Aesthetic Lab's site on Cloud Run (Dockerfile).
//
// It listens before anything else: no network I/O happens before the port
// is open, so a cold instance answers its startup probe at once. Content
// comes from the Firestore database FIRESTORE_DATABASE names, in
// GOOGLE_CLOUD_PROJECT (else the project the credentials belong to), or,
// without one, from the embedded snapshot.
package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"cloud.google.com/go/firestore"

	"github.com/lieranderl/aestheticlab/internal/content"
	"github.com/lieranderl/aestheticlab/internal/server"
	"github.com/lieranderl/aestheticlab/static"
)

func main() {
	// Cloud Run sends SIGTERM and allows 10 seconds before it stops the
	// instance.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Getenv, os.Stdout); err != nil {
		stop()
		os.Exit(1)
	}
}

// shutdownTimeout leaves in-flight requests most of Cloud Run's 10 seconds.
const shutdownTimeout = 8 * time.Second

// run serves the site on PORT until ctx is done, then shuts down gracefully.
// It logs to logOut and returns the error that stopped it, if any.
func run(ctx context.Context, getenv func(string) string, logOut io.Writer) error {
	log := server.NewLogger(logOut)
	port, err := listenPort(getenv)
	if err != nil {
		log.Error("server_config_rejected", "error", err.Error())
		return err
	}

	listener, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		log.Error("server_failed", "error", err.Error())
		return err
	}
	store, closeStore := openContent(ctx, getenv, log)
	defer closeStore()
	// The first request joins this fetch, made while Cloud Run still
	// allocates CPU to the starting instance.
	store.Prefetch()

	srv := &http.Server{
		Handler:           server.New(server.Config{Static: static.Files, Logger: log, Content: store, PlainHTTP: plainHTTP(getenv)}),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	shutdownDone := make(chan error, 1)
	go func() {
		<-ctx.Done()
		log.Info("server_stopping")
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
		defer cancel()
		shutdownDone <- srv.Shutdown(shutdownCtx)
	}()

	log.Info("server_started", "port", port)
	if err := srv.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
		log.Error("server_failed", "error", err.Error())
		return err
	}
	if err := <-shutdownDone; err != nil {
		log.Error("server_shutdown_failed", "error", err.Error())
		return err
	}
	log.Info("server_stopped")
	return nil
}

// openContent is the content store over the Firestore database the
// environment names, or over the snapshot alone when it names none or the
// client cannot be created. The client connects on its first read.
func openContent(ctx context.Context, getenv func(string) string, log *slog.Logger) (*content.Store, func()) {
	database := getenv("FIRESTORE_DATABASE")
	if database == "" {
		log.Warn("content_database_not_configured")
		return content.NewStore(content.Options{Logger: log}), func() {}
	}
	project := cmp.Or(getenv("GOOGLE_CLOUD_PROJECT"), firestore.DetectProjectID)
	db, err := content.NewFirestore(ctx, project, database)
	if err != nil {
		log.Error("content_database_failed", "database", database, "error", err.Error())
		return content.NewStore(content.Options{Logger: log}), func() {}
	}
	return content.NewStore(content.Options{Source: db, Logger: log}), func() { _ = db.Close() }
}

// plainHTTP reports whether browsers reach this server without TLS: Cloud
// Run, which always sets K_SERVICE, serves HTTPS; make go-dev, a local
// binary and a local container do not.
func plainHTTP(getenv func(string) string) bool {
	return getenv("K_SERVICE") == ""
}

// listenPort is the port Cloud Run gives in PORT, else 3000 (the
// container's port).
func listenPort(getenv func(string) string) (int, error) {
	raw := getenv("PORT")
	if raw == "" {
		return 3000, nil
	}
	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("PORT %q is not a TCP port", raw)
	}
	return port, nil
}
