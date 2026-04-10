package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"crypto-ingest/internal/model"
)

type BanHandler func()

type BinanceClient struct {
	baseURL       string
	httpClient    *http.Client
	rateLimiter   *RateLimiter
	maxRetries    int
	retryDelay    time.Duration
	banned        atomic.Bool
	banHandler    BanHandler
	mu            sync.Mutex
	pauseUntil    time.Time
}

func NewBinanceClient(baseURL string, rl *RateLimiter, maxRetries int, retryDelay time.Duration) *BinanceClient {
	return &BinanceClient{
		baseURL:     baseURL,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		rateLimiter: rl,
		maxRetries:  maxRetries,
		retryDelay:  retryDelay,
	}
}

func (c *BinanceClient) SetBanHandler(h BanHandler) {
	c.banHandler = h
}

func (c *BinanceClient) IsBanned() bool {
	return c.banned.Load()
}

func (c *BinanceClient) Ping(ctx context.Context) error {
	url := c.baseURL + "/api/v3/ping"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating ping request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf(
			"binance ping failed: %w\n"+
				"alternative base URLs: api1.binance.com, api2.binance.com, api3.binance.com\n"+
				"set BINANCE_BASE_URL to one of: https://api1.binance.com, https://api2.binance.com, https://api3.binance.com\n"+
				"if geo-blocked, set HTTP_PROXY/HTTPS_PROXY environment variables", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf(
			"binance ping returned status %d\n"+
				"alternative base URLs: api1.binance.com, api2.binance.com, api3.binance.com\n"+
				"set BINANCE_BASE_URL to override\n"+
				"if geo-blocked, set HTTP_PROXY/HTTPS_PROXY environment variables", resp.StatusCode)
	}
	return nil
}

func (c *BinanceClient) FetchExchangeInfo(ctx context.Context, symbols []string) ([]model.Symbol, error) {
	quoted := make([]string, len(symbols))
	for i, s := range symbols {
		quoted[i] = fmt.Sprintf("%q", s)
	}
	symbolsParam := "[" + strings.Join(quoted, ",") + "]"
	url := c.baseURL + "/api/v3/exchangeInfo?symbols=" + symbolsParam

	body, err := c.doRequest(ctx, url, 20)
	if err != nil {
		return nil, err
	}
	if body == nil {
		return nil, fmt.Errorf("empty response from exchangeInfo")
	}

	var raw struct {
		Symbols []struct {
			Symbol              string `json:"symbol"`
			Status              string `json:"status"`
			BaseAsset           string `json:"baseAsset"`
			QuoteAsset          string `json:"quoteAsset"`
			BaseAssetPrecision  int    `json:"baseAssetPrecision"`
			QuotePrecision      int    `json:"quotePrecision"`
		} `json:"symbols"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		logRawResponse("exchangeInfo", body)
		return nil, fmt.Errorf("unmarshaling exchangeInfo: %w", err)
	}

	result := make([]model.Symbol, 0, len(raw.Symbols))
	for _, s := range raw.Symbols {
		if s.Symbol == "" || s.Status == "" {
			slog.Warn("skipping symbol with empty fields", "symbol", s.Symbol)
			continue
		}
		result = append(result, model.Symbol{
			Symbol:         s.Symbol,
			Status:         s.Status,
			BaseAsset:      s.BaseAsset,
			QuoteAsset:     s.QuoteAsset,
			BasePrecision:  s.BaseAssetPrecision,
			QuotePrecision: s.QuotePrecision,
		})
	}
	return result, nil
}

func (c *BinanceClient) FetchAggTrades(ctx context.Context, symbol string, fromID *int64) ([]model.AggTrade, error) {
	url := fmt.Sprintf("%s/api/v3/aggTrades?symbol=%s&limit=1000", c.baseURL, symbol)
	if fromID != nil {
		url += fmt.Sprintf("&fromId=%d", *fromID)
	}

	body, err := c.doRequest(ctx, url, 2)
	if err != nil {
		return nil, err
	}
	if body == nil {
		return nil, nil
	}

	var raw []struct {
		AggTradeID   int64  `json:"a"`
		Price        string `json:"p"`
		Quantity     string `json:"q"`
		FirstTradeID int64  `json:"f"`
		LastTradeID  int64  `json:"l"`
		Timestamp    int64  `json:"T"`
		IsBuyerMaker bool   `json:"m"`
		IsBestMatch  bool   `json:"M"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		logRawResponse("aggTrades", body)
		return nil, fmt.Errorf("unmarshaling aggTrades: %w", err)
	}

	trades := make([]model.AggTrade, 0, len(raw))
	for _, t := range raw {
		if t.Price == "" || t.Quantity == "" {
			slog.Warn("skipping trade with empty price/quantity", "symbol", symbol, "agg_trade_id", t.AggTradeID)
			continue
		}
		trades = append(trades, model.AggTrade{
			Symbol:       symbol,
			AggTradeID:   t.AggTradeID,
			Price:        t.Price,
			Quantity:     t.Quantity,
			FirstTradeID: t.FirstTradeID,
			LastTradeID:  t.LastTradeID,
			TradeTime:    time.UnixMilli(t.Timestamp),
			IsBuyerMaker: t.IsBuyerMaker,
			IsBestMatch:  t.IsBestMatch,
		})
	}
	return trades, nil
}

func (c *BinanceClient) FetchOrderBook(ctx context.Context, symbol string, depth int) (*model.OrderBook, error) {
	url := fmt.Sprintf("%s/api/v3/depth?symbol=%s&limit=%d", c.baseURL, symbol, depth)

	body, err := c.doRequest(ctx, url, 2)
	if err != nil {
		return nil, err
	}
	if body == nil {
		return nil, nil
	}

	var raw struct {
		LastUpdateID int64      `json:"lastUpdateId"`
		Bids         [][]string `json:"bids"`
		Asks         [][]string `json:"asks"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		logRawResponse("depth", body)
		return nil, fmt.Errorf("unmarshaling depth: %w", err)
	}

	ob := &model.OrderBook{
		Symbol:       symbol,
		LastUpdateID: raw.LastUpdateID,
		DepthLevel:   depth,
		Bids:         parseLevels(raw.Bids, symbol, "bid"),
		Asks:         parseLevels(raw.Asks, symbol, "ask"),
	}
	return ob, nil
}

func (c *BinanceClient) FetchTicker24hr(ctx context.Context, symbol string) (*model.Ticker24hr, error) {
	url := fmt.Sprintf("%s/api/v3/ticker/24hr?symbol=%s", c.baseURL, symbol)

	body, err := c.doRequest(ctx, url, 2)
	if err != nil {
		return nil, err
	}
	if body == nil {
		return nil, nil
	}

	var raw struct {
		Symbol             string `json:"symbol"`
		PriceChange        string `json:"priceChange"`
		PriceChangePercent string `json:"priceChangePercent"`
		WeightedAvgPrice   string `json:"weightedAvgPrice"`
		PrevClosePrice     string `json:"prevClosePrice"`
		LastPrice          string `json:"lastPrice"`
		LastQty            string `json:"lastQty"`
		BidPrice           string `json:"bidPrice"`
		BidQty             string `json:"bidQty"`
		AskPrice           string `json:"askPrice"`
		AskQty             string `json:"askQty"`
		OpenPrice          string `json:"openPrice"`
		HighPrice          string `json:"highPrice"`
		LowPrice           string `json:"lowPrice"`
		Volume             string `json:"volume"`
		QuoteVolume        string `json:"quoteVolume"`
		OpenTime           int64  `json:"openTime"`
		CloseTime          int64  `json:"closeTime"`
		FirstID            int64  `json:"firstId"`
		LastID             int64  `json:"lastId"`
		Count              int64  `json:"count"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		logRawResponse("ticker24hr", body)
		return nil, fmt.Errorf("unmarshaling ticker24hr: %w", err)
	}

	if raw.LastPrice == "" || raw.Volume == "" {
		return nil, fmt.Errorf("ticker response has empty required fields for %s", symbol)
	}

	return &model.Ticker24hr{
		Symbol:         symbol,
		PriceChange:    raw.PriceChange,
		PriceChangePct: raw.PriceChangePercent,
		WeightedAvg:    raw.WeightedAvgPrice,
		PrevClose:      raw.PrevClosePrice,
		LastPrice:      raw.LastPrice,
		LastQty:        raw.LastQty,
		BidPrice:       raw.BidPrice,
		BidQty:         raw.BidQty,
		AskPrice:       raw.AskPrice,
		AskQty:         raw.AskQty,
		OpenPrice:      raw.OpenPrice,
		HighPrice:      raw.HighPrice,
		LowPrice:       raw.LowPrice,
		Volume:         raw.Volume,
		QuoteVolume:    raw.QuoteVolume,
		OpenTime:       time.UnixMilli(raw.OpenTime),
		CloseTime:      time.UnixMilli(raw.CloseTime),
		FirstTradeID:   raw.FirstID,
		LastTradeID:    raw.LastID,
		TradeCount:     raw.Count,
	}, nil
}

func (c *BinanceClient) doRequest(ctx context.Context, url string, weight int) ([]byte, error) {
	if c.banned.Load() {
		return nil, fmt.Errorf("binance client is banned")
	}

	c.mu.Lock()
	pauseUntil := c.pauseUntil
	c.mu.Unlock()
	if time.Now().Before(pauseUntil) {
		waitDur := time.Until(pauseUntil)
		slog.Info("rate limit pause active, waiting", "duration", waitDur)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(waitDur):
		}
	}

	if err := c.rateLimiter.Wait(ctx, weight); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	var lastErr error
	for attempt := range c.maxRetries + 1 {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = err
			c.backoff(ctx, attempt)
			continue
		}

		c.checkServerWeight(resp)

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = err
			c.backoff(ctx, attempt)
			continue
		}

		switch {
		case resp.StatusCode == http.StatusOK:
			return body, nil
		case resp.StatusCode == http.StatusTeapot:
			slog.Error("received HTTP 418 IP ban from binance", "url", url, "body", truncate(string(body), 500))
			c.banned.Store(true)
			if c.banHandler != nil {
				c.banHandler()
			}
			return nil, fmt.Errorf("HTTP 418: IP banned by binance")
		case resp.StatusCode == http.StatusTooManyRequests:
			retryAfter := resp.Header.Get("Retry-After")
			slog.Warn("rate limited by binance", "status", 429, "retry_after", retryAfter, "attempt", attempt+1)
			if retryAfter != "" {
				if secs, err := strconv.Atoi(retryAfter); err == nil {
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					case <-time.After(time.Duration(secs) * time.Second):
					}
					lastErr = fmt.Errorf("HTTP 429: rate limited")
					continue
				}
			}
			lastErr = fmt.Errorf("HTTP 429: rate limited")
			c.backoff(ctx, attempt)
			continue
		case resp.StatusCode == http.StatusBadRequest:
			slog.Warn("bad request to binance", "status", 400, "url", url, "body", truncate(string(body), 500))
			return nil, nil
		case resp.StatusCode >= 500:
			slog.Warn("binance server error", "status", resp.StatusCode, "attempt", attempt+1)
			lastErr = fmt.Errorf("HTTP %d from binance", resp.StatusCode)
			c.backoff(ctx, attempt)
			continue
		default:
			slog.Warn("unexpected status from binance", "status", resp.StatusCode, "body", truncate(string(body), 500))
			return nil, fmt.Errorf("unexpected HTTP %d", resp.StatusCode)
		}
	}
	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

func (c *BinanceClient) checkServerWeight(resp *http.Response) {
	weightStr := resp.Header.Get("X-MBX-USED-WEIGHT-1m")
	if weightStr == "" {
		return
	}
	weight, err := strconv.Atoi(weightStr)
	if err != nil {
		return
	}

	if weight >= 5700 {
		slog.Error("binance server-side rate at 95%, pausing all requests", "used_weight", weight)
		c.mu.Lock()
		c.pauseUntil = time.Now().Add(60 * time.Second)
		c.mu.Unlock()
	} else if weight >= 4800 {
		slog.Warn("binance server-side rate at 80%", "used_weight", weight)
	}
}

func (c *BinanceClient) backoff(ctx context.Context, attempt int) {
	delay := c.retryDelay
	for range attempt {
		delay *= 2
	}
	jittered := time.Duration(float64(delay) * (0.5 + rand.Float64()))
	select {
	case <-ctx.Done():
	case <-time.After(jittered):
	}
}

func parseLevels(raw [][]string, symbol, side string) []model.OrderBookLevel {
	levels := make([]model.OrderBookLevel, 0, len(raw))
	for _, entry := range raw {
		if len(entry) < 2 {
			slog.Warn("skipping orderbook level with insufficient data", "symbol", symbol, "side", side)
			continue
		}
		if entry[0] == "" || entry[1] == "" {
			slog.Warn("skipping orderbook level with empty price/quantity", "symbol", symbol, "side", side)
			continue
		}
		levels = append(levels, model.OrderBookLevel{
			Price:    entry[0],
			Quantity: entry[1],
		})
	}
	return levels
}

func logRawResponse(endpoint string, body []byte) {
	slog.Error("failed to unmarshal response", "endpoint", endpoint, "raw_response", truncate(string(body), 500))
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}
