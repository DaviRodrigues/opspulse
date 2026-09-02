package models

import "time"

type CheckResult struct {
	URL string
	StatusCode int
	Latency time.Duration
	IsUp bool
	Error error
}