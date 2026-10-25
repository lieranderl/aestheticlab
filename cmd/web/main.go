// Command web is Aesthetic Lab's site on Cloud Run (Dockerfile.web).
//
// It listens before anything else: no network I/O happens before
// ListenAndServe, so a cold instance answers its startup probe at once.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

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

	srv := &http.Server{
		Addr:              ":" + strconv.Itoa(port),
		Handler:           server.New(server.Config{Static: static.Files, Logger: log}),
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
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
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
