package model

import "time"

type Ticker24hr struct {
	Symbol         string
	PriceChange    string
	PriceChangePct string
	WeightedAvg    string
	PrevClose      string
	LastPrice      string
	LastQty        string
	BidPrice       string
	BidQty         string
	AskPrice       string
	AskQty         string
	OpenPrice      string
	HighPrice      string
	LowPrice       string
	Volume         string
	QuoteVolume    string
	OpenTime       time.Time
	CloseTime      time.Time
	FirstTradeID   int64
	LastTradeID    int64
	TradeCount     int64
}
