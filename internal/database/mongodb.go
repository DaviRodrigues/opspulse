package database

import (
	"context"

	"github.com/DaviRodrigues/opspulse/internal/config"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type MongoClient struct {
	client   *mongo.Client
	database *mongo.Database
}

func NewMongoClient(ctx context.Context, cfgMongo config.DatabaseConfig) (*MongoClient, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, cfgMongo.Timeout)
	defer cancel()

	opts := options.Client().ApplyURI(cfgMongo.URI)
	client, err := mongo.Connect(opts)
	if err != nil {
		return nil, err
	}

	if err := client.Ping(ctxTimeout, nil); err != nil {
		return nil, err
	}

	return &MongoClient{
		client:   client,
		database: client.Database(cfgMongo.Name),
	}, nil
}

func (c *MongoClient) Database() *mongo.Database {
	return c.database
}
func (c *MongoClient) Close(ctx context.Context) error {
	return c.client.Disconnect(ctx)
}
