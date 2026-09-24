package api

import (
	"github.com/DaviRodrigues/opspulse/internal/checker"
)

type Event struct {
	ID    string `json:"id,omitempty"`
	Name  string `json:"name"`
	Data  any    `json:"data"`
	Retry int    `json:"retry,omitempty"`
}

func NewStatusEvent(results []checker.CheckResult) Event {
	return Event{
		Name:  "status",
		Data:  results,
		Retry: 5000,
	}
}

func NewAlertEvent(result checker.CheckResult) Event {
	return Event{
		Name:  "alert",
		Data:  result,
		Retry: 3000,
	}
}
