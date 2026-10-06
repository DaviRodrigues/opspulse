package database

import (
	"context"

	"github.com/DaviRodrigues/opspulse/internal/domain"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const CheckLogCollectionName = "check_logs"
const TargetCollectionName = "targets"

type CheckLogRepository interface {
	Create(ctx context.Context, checkLog *domain.CheckLog) error
	FindByID(ctx context.Context, id bson.ObjectID) (*domain.CheckLog, error)
	FindAll(ctx context.Context, limit, offset int64) ([]*domain.CheckLog, error)
}

type MongoDBCheckLogRepository struct {
	collection *mongo.Collection
}

func (check *MongoDBCheckLogRepository) Create(ctx context.Context, checkLog *domain.CheckLog) error

func (check *MongoDBCheckLogRepository) FindByID(ctx context.Context, id string) (*domain.CheckLog, error)

func (check *MongoDBCheckLogRepository) FindAll(ctx context.Context, limit, offset int64) ([]*domain.CheckLog, error)

type TargetRepository interface {
	Create(ctx context.Context, target *domain.Target) error
	FindByID(ctx context.Context, id bson.ObjectID) (*domain.Target, error)
	FindAll(ctx context.Context) ([]*domain.Target, error)
	Update(ctx context.Context, target *domain.Target) error
	Delete(ctx context.Context) error
}

type MongoDBTargetRepository struct {
	collection *mongo.Collection
}

func (tar *MongoDBTargetRepository) Create(ctx context.Context, target *domain.Target) error

func (tar *MongoDBTargetRepository) FindByID(ctx context.Context, id bson.ObjectID) (*domain.Target, error)

func (tar *MongoDBTargetRepository) FindAll(ctx context.Context) ([]*domain.Target, error)

func (tar *MongoDBTargetRepository) Update(ctx context.Context, target *domain.Target) error

func (tar *MongoDBTargetRepository) Delete(ctx context.Context) error
