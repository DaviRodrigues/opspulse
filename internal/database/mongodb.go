package database

import (
	"context"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/config"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const defaultMongoTimeout = 5 * time.Second

type MongoClient struct {
	client   *mongo.Client
	database *mongo.Database
}

func NewMongoClient(ctx context.Context, cfgMongo config.DatabaseConfig) (*MongoClient, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	timeout := cfgMongo.Timeout
	if timeout <= 0 {
		timeout = defaultMongoTimeout
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	opts := options.Client().ApplyURI(cfgMongo.URI)
	client, err := mongo.Connect(opts)
	if err != nil {
		return nil, err
	}

	if err := client.Ping(ctxTimeout, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}

	return &MongoClient{
		client:   client,
		database: client.Database(cfgMongo.Name),
	}, nil
}

func (c *MongoClient) Database() *mongo.Database {
	if c == nil {
		return nil
	}
	return c.database
}

func (c *MongoClient) Close(ctx context.Context) error {
	if c == nil || c.client == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return c.client.Disconnect(ctx)
}
