// Command tenttiarkisto-server runs the web application.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/JaniL/tenttiarkisto/internal/config"
	"github.com/JaniL/tenttiarkisto/internal/db"
	"github.com/JaniL/tenttiarkisto/internal/geogate"
	"github.com/JaniL/tenttiarkisto/internal/models"
	"github.com/JaniL/tenttiarkisto/internal/storage"
	"github.com/JaniL/tenttiarkisto/internal/web"
)

var version = "dev"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config", "err", err)
		os.Exit(1)
	}

	ctx := context.Background()

	pool, err := db.Connect(ctx, cfg.DatabaseURL, logger)
	if err != nil {
		logger.Error("db connect", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		logger.Error("migrate", "err", err)
		os.Exit(1)
	}

	gate, err := geogate.New(cfg.GeoGateEnabled, cfg.TrustedIPHeader, cfg.GeoGateExtraCIDR, logger)
	if err != nil {
		logger.Error("geogate", "err", err)
		os.Exit(1)
	}
	logger.Info("geogate", "enabled", gate.Enabled())

	s3, err := storage.New(ctx, cfg.S3Endpoint, cfg.S3Region, cfg.S3Bucket, cfg.S3AccessKey, cfg.S3SecretKey)
	if err != nil {
		logger.Error("s3 client", "err", err)
		os.Exit(1)
	}

	store := models.NewStore(pool)
	app, err := web.New(cfg, store, gate, s3, logger)
	if err != nil {
		logger.Error("web", "err", err)
		os.Exit(1)
	}

	if _, err := cfg.PortInt(); err != nil {
		logger.Error("port", "err", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           app.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       5 * time.Minute, // allow large uploads
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       60 * time.Second,
	}

	logger.Info("listening", "addr", srv.Addr, "version", version,
		"bucket", cfg.S3Bucket, "endpoint", cfg.S3Endpoint)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("serve", "err", err)
		os.Exit(1)
	}
}
