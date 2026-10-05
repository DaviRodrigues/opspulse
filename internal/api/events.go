package api

import (
	"github.com/DaviRodrigues/opspulse/internal/domain"
)

func NewStatusEvent(results []domain.CheckLog) domain.Event {
	return domain.Event{
		Name:  "status",
		Data:  results,
		Retry: 30000,
	}
}

func NewAlertEvent(result domain.CheckLog) domain.Event {
	return domain.Event{
		Name:  "alert",
		Data:  result,
		Retry: 30000,
	}
}
