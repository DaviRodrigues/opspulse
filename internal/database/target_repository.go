package database

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/domain"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
)

type MongoDBTargetRepository struct {
	collection     *mongo.Collection
	indexesEnsured atomic.Bool
	indexesMu      sync.Mutex
}

func NewMongoDBTargetRepository(db *mongo.Database) *MongoDBTargetRepository {
	if db == nil {
		return nil
	}

	opts := options.Collection().SetWriteConcern(writeconcern.Majority())
	return &MongoDBTargetRepository{
		collection: db.Collection(TargetCollectionName, opts),
	}
}

func (r *MongoDBTargetRepository) EnsureIndexes(ctx context.Context) error {
	if r == nil || r.collection == nil {
		return domain.ErrInvalidInput
	}
	if r.indexesEnsured.Load() {
		return nil
	}

	r.indexesMu.Lock()
	defer r.indexesMu.Unlock()

	if r.indexesEnsured.Load() {
		return nil
	}

	models := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "url", Value: 1}},
			Options: options.Index().SetName("idx_targets_url_unique").SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "created_at", Value: -1}},
			Options: options.Index().SetName("idx_targets_created_at"),
		},
	}

	if _, err := r.collection.Indexes().CreateMany(ctx, models); err != nil {
		return err
	}

	r.indexesEnsured.Store(true)
	return nil
}

func buildTargetBSONFilter(filter TargetFilter) bson.D {
	clauses := bson.D{}

	if filter.Enabled != nil {
		clauses = append(clauses, bson.E{Key: "enabled", Value: *filter.Enabled})
	}
	if tag := strings.TrimSpace(filter.Tag); tag != "" {
		clauses = append(clauses, bson.E{Key: "tags", Value: tag})
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		pattern := regexp.QuoteMeta(search)
		regex := bson.M{"$regex": pattern, "$options": "i"}
		clauses = append(clauses, bson.E{
			Key: "$or",
			Value: bson.A{
				bson.M{"name": regex},
				bson.M{"url": regex},
			},
		})
	}

	return clauses
}

func (r *MongoDBTargetRepository) Create(ctx context.Context, target *domain.Target) error {
	if r == nil || r.collection == nil || target == nil {
		return domain.ErrInvalidInput
	}

	if err := r.EnsureIndexes(ctx); err != nil {
		return err
	}

	if target.ID.IsZero() {
		target.ID = bson.NewObjectID()
	}
	normalizeTargetDefaults(target)

	now := time.Now().UTC()
	if target.CreatedAt.IsZero() {
		target.CreatedAt = now
	} else {
		target.CreatedAt = target.CreatedAt.UTC()
	}
	target.UpdatedAt = now

	if _, err := r.collection.InsertOne(ctx, target); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return domain.ErrDuplicateURL
		}
		return err
	}

	return nil
}

func (r *MongoDBTargetRepository) FindByID(ctx context.Context, id bson.ObjectID) (*domain.Target, error) {
	if r == nil || r.collection == nil {
		return nil, domain.ErrInvalidInput
	}

	var target domain.Target
	err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&target)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}

	return &target, nil
}

func (r *MongoDBTargetRepository) FindAll(ctx context.Context, filter TargetFilter, limit, offset int64) ([]*domain.Target, int64, error) {
	if r == nil || r.collection == nil {
		return nil, 0, domain.ErrInvalidInput
	}

	bsonFilter := buildTargetBSONFilter(filter)

	total, err := r.collection.CountDocuments(ctx, bsonFilter)
	if err != nil {
		return nil, 0, err
	}

	findOpts := options.Find().SetSort(bson.D{
		{Key: "created_at", Value: -1},
		{Key: "_id", Value: -1},
	})
	if offset > 0 {
		findOpts.SetSkip(offset)
	}
	if limit > 0 {
		findOpts.SetLimit(limit)
	}

	cursor, err := r.collection.Find(ctx, bsonFilter, findOpts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	targets := make([]*domain.Target, 0)
	if err := cursor.All(ctx, &targets); err != nil {
		return nil, 0, err
	}

	return targets, total, nil
}

func (r *MongoDBTargetRepository) FindEnabled(ctx context.Context) ([]*domain.Target, error) {
	if r == nil || r.collection == nil {
		return nil, domain.ErrInvalidInput
	}

	findOpts := options.Find().SetSort(bson.D{
		{Key: "created_at", Value: -1},
		{Key: "_id", Value: -1},
	})

	cursor, err := r.collection.Find(ctx, bson.D{{Key: "enabled", Value: true}}, findOpts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	targets := make([]*domain.Target, 0)
	if err := cursor.All(ctx, &targets); err != nil {
		return nil, err
	}

	return targets, nil
}

func (r *MongoDBTargetRepository) Update(ctx context.Context, id bson.ObjectID, target *domain.Target) error {
	if r == nil || r.collection == nil || target == nil {
		return domain.ErrInvalidInput
	}

	normalizeTargetDefaults(target)
	now := time.Now().UTC()

	updateDoc := bson.D{
		{
			Key: "$set",
			Value: bson.D{
				{Key: "name", Value: target.Name},
				{Key: "url", Value: target.URL},
				{Key: "method", Value: target.Method},
				{Key: "headers", Value: target.Headers},
				{Key: "enabled", Value: target.Enabled},
				{Key: "tags", Value: target.Tags},
				{Key: "body", Value: target.Body},
				{Key: "timeout", Value: target.Timeout},
				{Key: "interval", Value: target.Interval},
				{Key: "expected_status", Value: target.ExpectedStatus},
				{Key: "expected_body_match", Value: target.ExpectedBodyMatch},
				{Key: "updated_at", Value: now},
			},
		},
	}

	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	err := r.collection.FindOneAndUpdate(ctx, bson.D{{Key: "_id", Value: id}}, updateDoc, opts).Decode(target)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return domain.ErrNotFound
		}
		if mongo.IsDuplicateKeyError(err) {
			return domain.ErrDuplicateURL
		}
		return err
	}

	return nil
}

func (r *MongoDBTargetRepository) Delete(ctx context.Context, id bson.ObjectID) error {
	if r == nil || r.collection == nil {
		return domain.ErrInvalidInput
	}

	res, err := r.collection.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return domain.ErrNotFound
	}

	return nil
}
