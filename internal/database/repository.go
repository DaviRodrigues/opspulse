package database

import (
	"context"
	"strings"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/domain"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	CheckLogCollectionName = "check_logs"
	TargetCollectionName   = "targets"

	defaultTargetMethod  = "GET"
	defaultTargetTimeout = 5 * time.Second
)

type TargetFilter struct {
	Enabled *bool
	Tag     string
	Search  string
}

type TargetRepository interface {
	Create(ctx context.Context, target *domain.Target) error
	FindByID(ctx context.Context, id bson.ObjectID) (*domain.Target, error)
	FindAll(ctx context.Context, filter TargetFilter, limit, offset int64) ([]*domain.Target, int64, error)
	FindEnabled(ctx context.Context) ([]*domain.Target, error)
	Update(ctx context.Context, id bson.ObjectID, target *domain.Target) error
	Delete(ctx context.Context, id bson.ObjectID) error
}

type CheckLogFilter struct {
	TargetID   *bson.ObjectID
	IsUp       *bool
	StatusCode *int
	From       *time.Time
	To         *time.Time
	Search     string
}

type CheckLogRepository interface {
	Create(ctx context.Context, checkLog *domain.CheckLog) error
	CreateBatch(ctx context.Context, logs []domain.CheckLog) error
	FindByID(ctx context.Context, id bson.ObjectID) (*domain.CheckLog, error)
	FindAll(ctx context.Context, filter CheckLogFilter, limit, offset int64) ([]domain.CheckLog, int64, error)
	FindByTargetID(ctx context.Context, targetID bson.ObjectID, limit, offset int64) ([]domain.CheckLog, int64, error)
}

func normalizeTargetDefaults(target *domain.Target) {
	method := strings.TrimSpace(target.Method)
	if method == "" {
		target.Method = defaultTargetMethod
	} else {
		target.Method = strings.ToUpper(method)
	}

	if target.Timeout <= 0 {
		target.Timeout = defaultTargetTimeout
	}
}
