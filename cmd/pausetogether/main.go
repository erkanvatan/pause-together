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
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/erkanvatan/pause-together/internal/api"
	"github.com/erkanvatan/pause-together/internal/store"
	"github.com/erkanvatan/pause-together/web"
)

type config struct {
	guestAddr string
	adminAddr string
	dataDir   string // database and backups; later the cache
}

func loadConfig() config {
	return config{
		guestAddr: envOr("GUEST_ADDR", ":8080"),
		adminAddr: envOr("ADMIN_ADDR", ":8081"),
		dataDir:   envOr("DATA_DIR", "/data"),
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

	db, err := store.Open(ctx, filepath.Join(cfg.dataDir, "pausetogether.db"), store.Migrations())
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			slog.Error("close database", "err", err)
		}
	}()

	backups := &store.Backups{
		DB:   db,
		Dir:  filepath.Join(cfg.dataDir, "backups"),
		Keep: store.BackupKeep,
		Now:  time.Now,
	}
	backupCtx, stopBackups := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Go(func() { backups.Run(backupCtx) })
	// Runs before db.Close: the backup loop must stop before the database closes.
	defer func() {
		stopBackups()
		wg.Wait()
	}()

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
