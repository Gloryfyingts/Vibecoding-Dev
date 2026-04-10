package worker

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"crypto-ingest/internal/client"
	"crypto-ingest/internal/config"
	"crypto-ingest/internal/store"
)

type Manager struct {
	cfg        *config.Config
	client     *client.BinanceClient
	store      *store.Store
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	healthy    atomic.Bool
	banReason  atomic.Value
	httpServer *http.Server
	banCh      chan struct{}
}

func NewManager(cfg *config.Config, c *client.BinanceClient, s *store.Store) *Manager {
	m := &Manager{
		cfg:    cfg,
		client: c,
		store:  s,
		banCh:  make(chan struct{}),
	}
	m.healthy.Store(true)

	c.SetBanHandler(func() {
		m.handleBan("IP banned by Binance (HTTP 418)")
	})

	return m
}

func (m *Manager) handleBan(reason string) {
	slog.Error("ban detected, stopping all workers", "reason", reason)
	m.healthy.Store(false)
	m.banReason.Store(reason)
	if m.cancel != nil {
		m.cancel()
	}
	select {
	case <-m.banCh:
	default:
		close(m.banCh)
	}
}

func (m *Manager) Start(ctx context.Context) error {
	workerCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel

	for _, symbol := range m.cfg.Symbols {
		m.wg.Add(3)

		tw := NewTradeWorker(symbol, m.cfg.TradePollInterval, m.client, m.store)
		go func() {
			defer m.wg.Done()
			tw.Run(workerCtx)
		}()

		obw := NewOrderBookWorker(symbol, m.cfg.OrderbookDepth, m.cfg.OrderbookPollInterval, m.client, m.store)
		go func() {
			defer m.wg.Done()
			obw.Run(workerCtx)
		}()

		tkw := NewTickerWorker(symbol, m.cfg.TickerPollInterval, m.client, m.store)
		go func() {
			defer m.wg.Done()
			tkw.Run(workerCtx)
		}()
	}

	slog.Info("all workers started",
		"symbols", m.cfg.Symbols,
		"trade_interval", m.cfg.TradePollInterval,
		"orderbook_interval", m.cfg.OrderbookPollInterval,
		"ticker_interval", m.cfg.TickerPollInterval,
	)

	go m.startHealthServer()

	return nil
}

func (m *Manager) startHealthServer() {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if m.healthy.Load() {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "ok")
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		reason, _ := m.banReason.Load().(string)
		if reason == "" {
			reason = "unhealthy"
		}
		fmt.Fprint(w, reason)
	})

	addr := fmt.Sprintf(":%d", m.cfg.HealthPort)
	m.httpServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	slog.Info("health endpoint listening", "addr", addr)
	if err := m.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("health server error", "error", err)
	}
}

func (m *Manager) Stop() {
	slog.Info("shutting down workers")

	if m.cancel != nil {
		m.cancel()
	}

	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		slog.Info("all workers stopped gracefully")
	case <-time.After(10 * time.Second):
		slog.Warn("worker shutdown deadline exceeded")
	}

	if m.httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		m.httpServer.Shutdown(shutdownCtx)
	}
}

func (m *Manager) WaitForBanOrContext(ctx context.Context) {
	select {
	case <-ctx.Done():
	case <-m.banCh:
	}
}

func CheckHealth(port int) error {
	url := fmt.Sprintf("http://127.0.0.1:%d/healthz", port)
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}
	return nil
}
