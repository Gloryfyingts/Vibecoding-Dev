package model

import "time"

type OrderBook struct {
	Symbol       string
	LastUpdateID int64
	DepthLevel   int
	Bids         []OrderBookLevel
	Asks         []OrderBookLevel
}

type OrderBookLevel struct {
	Price    string
	Quantity string
}

type OrderBookSnapshot struct {
	SnapshotID   int64
	Symbol       string
	LastUpdateID int64
	DepthLevel   int
	SnapshotTime time.Time
}
