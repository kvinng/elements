package main

import (
	"context"
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/kving/games/elements/server/internal/auth"
	servernet "github.com/kving/games/elements/server/internal/net"
	"github.com/kving/games/elements/server/internal/store"
	"github.com/kving/games/elements/server/internal/world"
)

//go:embed static
var staticFiles embed.FS

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	// ── Store ─────────────────────────────────────────────────────────────────
	// DB_DSN controls the database. Default: SQLite file next to the binary.
	// PostgreSQL example: export DB_DSN="postgres://user:pass@host/db"
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		dsn = "file:./elements.db"
	}
	st, err := store.Open("sqlite", dsn)
	if err != nil {
		slog.Error("failed to open store", "err", err)
		os.Exit(1)
	}
	defer st.Close()
	slog.Info("store opened", "dsn", dsn)

	// ── Auth service ──────────────────────────────────────────────────────────
	// JWT_SECRET must be set in production. All game servers sharing a DB must
	// use the same secret so tokens issued by one are accepted by all others.
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		slog.Warn("JWT_SECRET not set — using insecure default, CHANGE IN PRODUCTION")
		jwtSecret = "elements-insecure-dev-secret"
	}
	authSvc := auth.NewHS256(jwtSecret)
	authHandler := servernet.NewAuthHandler(st, authSvc)

	// ── World + Hub ───────────────────────────────────────────────────────────
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	zone := world.NewDungeon(43) // 43%3=1 → lava biome
	hub := servernet.NewHub(zone, st, authSvc)

	go zone.Run(ctx)
	go hub.Run(ctx)

	// ── Routes ────────────────────────────────────────────────────────────────
	staticFS, _ := fs.Sub(staticFiles, "static")

	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/register", authHandler.Register)
	mux.HandleFunc("/api/auth/login", authHandler.Login)
	mux.HandleFunc("/ws", hub.ServeWS)
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	srv := &http.Server{Addr: ":8080", Handler: mux}
	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background()) //nolint:errcheck
	}()

	slog.Info("server listening", "addr", "http://localhost"+srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("http server failed", "err", err)
		os.Exit(1)
	}
}
