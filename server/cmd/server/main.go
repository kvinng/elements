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

	servernet "github.com/kving/games/elements/server/internal/net"
	"github.com/kving/games/elements/server/internal/world"
)

//go:embed static
var staticFiles embed.FS

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	zone := world.New()
	hub := servernet.NewHub(zone)

	go zone.Run(ctx)
	go hub.Run(ctx)

	staticFS, _ := fs.Sub(staticFiles, "static")

	mux := http.NewServeMux()
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
