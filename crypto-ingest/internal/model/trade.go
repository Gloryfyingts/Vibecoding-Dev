package model

import "time"

type AggTrade struct {
	Symbol        string
	AggTradeID    int64
	Price         string
	Quantity      string
	FirstTradeID  int64
	LastTradeID   int64
	TradeTime     time.Time
	IsBuyerMaker  bool
	IsBestMatch   bool
}
