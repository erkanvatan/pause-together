// Command pausetogether serves the app on two ports: guest and admin.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/erkanvatan/pause-together/internal/api"
	"github.com/erkanvatan/pause-together/web"
)

type config struct {
	guestAddr string
	adminAddr string
}

func loadConfig() config {
	return config{
		guestAddr: envOr("GUEST_ADDR", ":8080"),
		adminAddr: envOr("ADMIN_ADDR", ":8081"),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("exit", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := loadConfig()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	build := web.Build()
	servers := []*http.Server{
		newServer(cfg.guestAddr, api.Guest(build)),
		newServer(cfg.adminAddr, api.Admin(build)),
	}

	// Bind both ports before serving, so a bind error (port taken, Tailscale IP not up) fails at once.
	listeners := make([]net.Listener, len(servers))
	for i, srv := range servers {
		ln, err := net.Listen("tcp", srv.Addr)
		if err != nil {
			for _, l := range listeners[:i] {
				_ = l.Close()
			}
			return err
		}
		listeners[i] = ln
	}

	errs := make(chan error, len(servers))
	for i, srv := range servers {
		slog.Info("listening", "addr", srv.Addr)
		go func() { errs <- srv.Serve(listeners[i]) }()
	}

	var err error
	select {
	case <-ctx.Done():
		slog.Info("shutting down")
	case err = <-errs:
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, srv := range servers {
		if serr := srv.Shutdown(shutdownCtx); serr != nil {
			err = errors.Join(err, serr)
		}
	}
	return err
}

// newServer sets no write timeout: it would cut off long video streams and WebSockets.
func newServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
}
