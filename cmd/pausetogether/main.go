// Command pausetogether serves the app on two ports: guest and admin.
package main

import (
	"context"
	"errors"
	"io/fs"
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
	"github.com/erkanvatan/pause-together/internal/library"
	"github.com/erkanvatan/pause-together/internal/media"
	"github.com/erkanvatan/pause-together/internal/room"
	"github.com/erkanvatan/pause-together/internal/store"
	"github.com/erkanvatan/pause-together/internal/user"
	"github.com/erkanvatan/pause-together/web"
)

// shutdownTimeout is how long the HTTP servers wait for open requests on shutdown. .air.toml's
// kill_delay must stay longer.
const shutdownTimeout = 5 * time.Second

// mediaDir is where the media folder is mounted in the container. Library paths are relative to it.
const mediaDir = "/media"

type config struct {
	guestAddr string
	adminAddr string
	dataDir   string // database, backups and the cache of prepared videos and subtitles
	// tokenCookie names the user cookie. Dev sets its own: cookies ignore the port, so dev and prod
	// on localhost would otherwise overwrite each other's.
	tokenCookie string
	// buildID tells open pages from another build to reload. Dev sets one fixed ID for Go and Vite,
	// or its pages would reload forever.
	buildID string
}

func loadConfig(build fs.FS) config {
	return config{
		guestAddr:   envOr("GUEST_ADDR", ":8080"),
		adminAddr:   envOr("ADMIN_ADDR", ":8081"),
		dataDir:     envOr("DATA_DIR", "/data"),
		tokenCookie: envOr("TOKEN_COOKIE", "pt_token"),
		buildID:     envOr("BUILD_ID", web.BuildID(build)),
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
	build := web.Build()
	cfg := loadConfig(build)
	if cfg.buildID == "" {
		return errors.New("no build ID: set BUILD_ID, or embed a web build")
	}
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
	// Prepared videos and converted subtitles.
	cacheDir := filepath.Join(cfg.dataDir, "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return err
	}
	jobs := media.NewJobs(cacheDir, db, media.FFmpeg{})

	scanner := &library.Scanner{DB: db, Root: mediaDir, Prober: media.FFprobe{},
		Subtitles: media.Subtitles{Dir: cacheDir}}
	scans := library.NewScans(scanner)
	// Without a watcher, libraries still follow the disk through the timed rescan.
	watcher, err := library.NewWatcher(scans)
	if err != nil {
		slog.Error("file watching off", "err", err)
	} else {
		scanner.Watcher = watcher
	}

	bgCtx, stopBackground := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Go(func() { backups.Run(bgCtx) })
	wg.Go(func() { scans.Run(bgCtx) })
	wg.Go(func() { scans.Every(bgCtx, library.RescanInterval) })
	wg.Go(func() { jobs.Run(bgCtx) })
	wg.Go(func() { jobs.CleanEvery(bgCtx, media.CleanInterval) })
	if watcher != nil {
		wg.Go(func() { watcher.Run(bgCtx) })
	}
	libraries := &library.Libraries{DB: db, Root: mediaDir}
	rooms := &room.Rooms{DB: db, Library: libraries, Jobs: jobs}
	hub := room.NewHub(bgCtx, rooms, cfg.buildID)
	// Runs before db.Close: the backup loop, the scans, the watcher and the room loops must stop before
	// the database closes. It also waits for a running prepare job to stop and delete its half-written
	// copy.
	defer func() {
		stopBackground()
		wg.Wait()
		hub.Wait()
	}()
	// Every library is scanned once at startup. If that can't start, the server still serves; the
	// admin page can rescan.
	if err := scans.RequestAll(ctx); err != nil {
		slog.Error("startup scan", "err", err)
	}

	deps := api.Deps{
		Users:       &user.Store{DB: db},
		TokenCookie: cfg.tokenCookie,
		Libraries:   libraries,
		Scans:       scans,
		Jobs:        jobs,
		Rooms:       rooms,
		Hub:         hub,
	}
	servers := []*http.Server{
		newServer(cfg.guestAddr, api.Guest(build, deps)),
		newServer(cfg.adminAddr, api.Admin(build, deps)),
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

	// Both at once, so each gets the whole timeout.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	serrs := make([]error, len(servers))
	var swg sync.WaitGroup
	for i, srv := range servers {
		swg.Go(func() {
			serrs[i] = srv.Shutdown(shutdownCtx)
			if errors.Is(serrs[i], context.DeadlineExceeded) {
				// A video stream never goes idle: a viewer mid-movie keeps its request open. Cut it off.
				slog.Info("closing open streams", "addr", srv.Addr)
				serrs[i] = srv.Close()
			}
		})
	}
	swg.Wait()
	return errors.Join(append(serrs, err)...)
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
