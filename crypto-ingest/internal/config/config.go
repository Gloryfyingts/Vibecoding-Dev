package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL          string
	BinanceBaseURL       string
	Symbols              []string
	TradePollInterval    time.Duration
	OrderbookPollInterval time.Duration
	TickerPollInterval   time.Duration
	OrderbookDepth       int
	HealthPort           int
	LogLevel             string
	MaxRetries           int
	RetryBaseDelay       time.Duration
	PGMaxConns           int32
	PGAcquireTimeout     time.Duration
}

func Load() (*Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	baseURL := envOrDefault("BINANCE_BASE_URL", "https://api.binance.com")
	symbolsRaw := envOrDefault("SYMBOLS", "BTCUSDT,ETHUSDT,ETHBTC")
	symbols := strings.Split(symbolsRaw, ",")
	for i := range symbols {
		symbols[i] = strings.TrimSpace(symbols[i])
	}

	tradePoll, err := parseDuration("TRADE_POLL_INTERVAL", "5s")
	if err != nil {
		return nil, err
	}

	obPoll, err := parseDuration("ORDERBOOK_POLL_INTERVAL", "5s")
	if err != nil {
		return nil, err
	}

	tickerPoll, err := parseDuration("TICKER_POLL_INTERVAL", "30s")
	if err != nil {
		return nil, err
	}

	obDepth, err := parseInt("ORDERBOOK_DEPTH", 20)
	if err != nil {
		return nil, err
	}

	healthPort, err := parseInt("HEALTH_PORT", 8085)
	if err != nil {
		return nil, err
	}

	logLevel := envOrDefault("LOG_LEVEL", "info")

	maxRetries, err := parseInt("MAX_RETRIES", 3)
	if err != nil {
		return nil, err
	}

	retryDelay, err := parseDuration("RETRY_BASE_DELAY", "1s")
	if err != nil {
		return nil, err
	}

	pgMaxConns, err := parseInt("PG_MAX_CONNS", 12)
	if err != nil {
		return nil, err
	}

	pgAcquireTimeout, err := parseDuration("PG_ACQUIRE_TIMEOUT", "5s")
	if err != nil {
		return nil, err
	}

	return &Config{
		DatabaseURL:          dbURL,
		BinanceBaseURL:       strings.TrimRight(baseURL, "/"),
		Symbols:              symbols,
		TradePollInterval:    tradePoll,
		OrderbookPollInterval: obPoll,
		TickerPollInterval:   tickerPoll,
		OrderbookDepth:       obDepth,
		HealthPort:           healthPort,
		LogLevel:             logLevel,
		MaxRetries:           maxRetries,
		RetryBaseDelay:       retryDelay,
		PGMaxConns:           int32(pgMaxConns),
		PGAcquireTimeout:     pgAcquireTimeout,
	}, nil
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func parseDuration(key, defaultVal string) (time.Duration, error) {
	raw := envOrDefault(key, defaultVal)
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid duration for %s: %q: %w", key, raw, err)
	}
	return d, nil
}

func parseInt(key string, defaultVal int) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return defaultVal, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid integer for %s: %q: %w", key, raw, err)
	}
	return v, nil
}
