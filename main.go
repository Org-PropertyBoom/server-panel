//go:build !ctl

package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ppt/server-panel/routes"
	"ppt/server-panel/services"
)

//go:embed all:client/build
var clientFS embed.FS

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	startup := services.NewStartupService().Startup()
	logger.Info(
		"runtime selected",
		"mode", startup.Mode,
		"user", startup.Username,
		"uid", startup.UID,
		"root", startup.IsRoot,
	)

	mux := http.NewServeMux()
	auth := services.NewAuthService()
	sessions := services.NewSessionService()

	var buildTime string
	var version string
	if data, err := clientFS.ReadFile("client/build/version.json"); err == nil {
		var v struct {
			Version   string `json:"version"`
			BuildTime string `json:"buildTime"`
		}
		if err := json.Unmarshal(data, &v); err == nil {
			version = v.Version
			buildTime = v.BuildTime
		}
	}
	updater := services.NewUpdateService(version, buildTime)
	settings, err := services.NewSettingsService()
	if err != nil {
		logger.Error("settings database could not be opened", "error", err)
		os.Exit(1)
	}

	vhostEngine := services.NewVhostEngineService(services.NewDataSourceService(), settings)

	routes.Register(mux, routes.Dependencies{
		Auth:        auth,
		ClientFS:    clientFS,
		Health:      services.NewHealthService(startup.Env),
		Sessions:    sessions,
		Startup:     startup,
		Update:      updater,
		System:      services.NewSystemService(),
		Settings:    settings,
		VhostEngine: vhostEngine,
	})

	srv := &http.Server{
		Addr:              startup.Addr,
		Handler:           logRequests(logger, mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server starting", "addr", startup.Addr, "env", startup.Env, "mode", startup.Mode)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("server shutdown failed", "error", err)
			os.Exit(1)
		}
		logger.Info("server stopped")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}
}

func logRequests(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		logger.Info("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start).String())
	})
}
