package api

import (
	"github.com/DaviRodrigues/opspulse/internal/checker"
	"github.com/DaviRodrigues/opspulse/internal/domain"
)

func NewStatusEvent(results []checker.CheckResult) domain.Event {
	return domain.Event{
		Name:  "status",
		Data:  results,
		Retry: 30000,
	}
}

func NewAlertEvent(result checker.CheckResult) domain.Event {
	return domain.Event{
		Name:  "alert",
		Data:  result,
		Retry: 30000,
	}
}
