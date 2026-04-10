package model

import "time"

type IngestState struct {
	WorkerType    string
	Symbol        string
	LastID        *int64
	LastTimestamp  *time.Time
}
