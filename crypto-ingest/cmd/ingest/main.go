package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"crypto-ingest/internal/client"
	"crypto-ingest/internal/config"
	"crypto-ingest/internal/store"
	"crypto-ingest/internal/worker"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "")
	flag.Parse()

	if *healthcheck {
		cfg, err := config.Load()
		if err != nil {
			os.Exit(1)
		}
		if err := worker.CheckHealth(cfg.HealthPort); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	setupLogger(cfg.LogLevel)

	slog.Info("starting crypto-ingest",
		"symbols", cfg.Symbols,
		"binance_url", cfg.BinanceBaseURL,
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := store.New(ctx, cfg.DatabaseURL, cfg.PGMaxConns, cfg.PGAcquireTimeout)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.RunMigrations(ctx); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	rl := client.NewRateLimiter(5000)
	binance := client.NewBinanceClient(cfg.BinanceBaseURL, rl, cfg.MaxRetries, cfg.RetryBaseDelay)

	if err := binance.Ping(ctx); err != nil {
		slog.Error("binance connectivity check failed", "error", err)
		os.Exit(1)
	}
	slog.Info("binance connectivity verified")

	symbols, err := binance.FetchExchangeInfo(ctx, cfg.Symbols)
	if err != nil {
		slog.Error("failed to fetch exchange info", "error", err)
		os.Exit(1)
	}

	if err := db.UpsertSymbols(ctx, symbols); err != nil {
		slog.Error("failed to upsert symbols", "error", err)
		os.Exit(1)
	}
	slog.Info("symbols loaded", "count", len(symbols))

	mgr := worker.NewManager(cfg, binance, db)
	if err := mgr.Start(ctx); err != nil {
		slog.Error("failed to start workers", "error", err)
		os.Exit(1)
	}

	mgr.WaitForBanOrContext(ctx)

	mgr.Stop()
	slog.Info("crypto-ingest shutdown complete")
}

func setupLogger(level string) {
	var logLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})
	slog.SetDefault(slog.New(handler))
}
