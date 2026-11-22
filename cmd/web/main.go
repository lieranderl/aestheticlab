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
	_ "time/tzdata" // Europe/Brussels for booking times, whatever the image ships.

	"cloud.google.com/go/firestore"

	"github.com/lieranderl/aestheticlab/internal/assistant"
	"github.com/lieranderl/aestheticlab/internal/availability"
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
		Handler: server.New(server.Config{
			Static: static.Files, Logger: log, Content: store, PlainHTTP: plainHTTP(getenv),
			Assistant: openAssistant(ctx, getenv, log),
		}),
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

// defaultChatModel and defaultChatLocation keep chat requests on Gemini
// 3.1 Flash-Lite at Vertex AI's EU endpoint (plans/ai-salon-assistant.md).
const (
	defaultChatModel    = "gemini-3.1-flash-lite"
	defaultChatLocation = "eu"
	// bookingLead is how far ahead the chat's first offered time is.
	bookingLead = 2 * time.Hour
)

// openAssistant is the chat assistant when CHAT_ENABLED is "true", else
// nil (no chat). CHAT_MODEL names the Gemini model, or "fake" for a
// scripted one (local development, browser tests). Free times come from
// the calendars AVAILABILITY_CALENDARS maps to staff, or, with
// AVAILABILITY_DEV "true", from a made-up calendar.
func openAssistant(ctx context.Context, getenv func(string) string, log *slog.Logger) *assistant.Assistant {
	if getenv("CHAT_ENABLED") != "true" {
		return nil
	}
	brussels, err := time.LoadLocation("Europe/Brussels")
	if err != nil {
		log.Error("chat_config_rejected", "error", err.Error())
		return nil
	}
	var model assistant.Model = assistant.Fake{}
	name := cmp.Or(getenv("CHAT_MODEL"), defaultChatModel)
	if name != "fake" {
		project := getenv("GOOGLE_CLOUD_PROJECT")
		if project == "" {
			log.Error("chat_config_rejected", "error", "GOOGLE_CLOUD_PROJECT is required for the chat model")
			return nil
		}
		gemini, err := assistant.NewGemini(ctx, project, cmp.Or(getenv("CHAT_LOCATION"), defaultChatLocation), name)
		if err != nil {
			log.Error("chat_model_failed", "model", name, "error", err.Error())
			return nil
		}
		model = gemini
	}
	return assistant.New(assistant.Options{Model: model, Finder: openFinder(ctx, getenv, brussels, log), Location: brussels})
}

// openFinder reads free times from the configured calendars; nil when none
// are configured, so the chat offers the Book button instead of times.
func openFinder(ctx context.Context, getenv func(string) string, loc *time.Location, log *slog.Logger) *availability.Finder {
	calendars, err := availability.ParseCalendars(getenv("AVAILABILITY_CALENDARS"))
	if err != nil {
		log.Error("availability_config_rejected", "error", err.Error())
		return nil
	}
	opts := availability.FinderOptions{Calendars: calendars, Location: loc, Lead: bookingLead}
	switch {
	case getenv("AVAILABILITY_DEV") == "true":
		opts.Source, opts.EveryStaff = availability.DevSource{Location: loc}, true
		log.Warn("availability_dev_calendar")
	case len(calendars) > 0:
		google, err := availability.NewGoogle(ctx)
		if err != nil {
			log.Error("availability_calendar_failed", "error", err.Error())
			return nil
		}
		opts.Source = google
		// Connect while Cloud Run still gives the starting instance CPU.
		go func() {
			warmCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			if err := google.Warm(warmCtx); err != nil {
				log.Warn("availability_warm_failed", "error", err.Error())
			}
		}()
	default:
		log.Info("availability_not_configured")
		return nil
	}
	log.Info("availability_on", "calendars", len(calendars))
	return availability.NewFinder(opts)
}
