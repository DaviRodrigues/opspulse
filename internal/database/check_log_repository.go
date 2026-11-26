package database

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/domain"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
)

const checkLogsTTLSeconds int64 = 2592000 // 30 days

type MongoDBCheckLogRepository struct {
	collection *mongo.Collection
}

func NewMongoDBCheckLogRepository(db *mongo.Database) *MongoDBCheckLogRepository {
	if db == nil {
		return nil
	}

	opts := options.Collection().SetWriteConcern(writeconcern.W1())
	return &MongoDBCheckLogRepository{
		collection: db.Collection(CheckLogCollectionName, opts),
	}
}

func EnsureCheckLogsCollection(ctx context.Context, db *mongo.Database) error {
	if db == nil {
		return domain.ErrInvalidInput
	}

	names, err := db.ListCollectionNames(ctx, bson.D{{Key: "name", Value: CheckLogCollectionName}})
	if err != nil {
		return err
	}
	if len(names) > 0 {
		return nil
	}

	tsOpts := options.TimeSeries().
		SetTimeField("checked_at").
		SetMetaField("target_id").
		SetGranularity("seconds")

	createOpts := options.CreateCollection().
		SetTimeSeriesOptions(tsOpts).
		SetExpireAfterSeconds(checkLogsTTLSeconds)

	if err := db.CreateCollection(ctx, CheckLogCollectionName, createOpts); err != nil {
		var cmdErr mongo.CommandError
		if errors.As(err, &cmdErr) && cmdErr.Code == 48 {
			return nil
		}
		return err
	}

	return nil
}

func buildCheckLogBSONFilter(filter CheckLogFilter) bson.D {
	clauses := bson.D{}

	if filter.TargetID != nil {
		clauses = append(clauses, bson.E{Key: "target_id", Value: *filter.TargetID})
	}
	if filter.IsUp != nil {
		clauses = append(clauses, bson.E{Key: "is_up", Value: *filter.IsUp})
	}
	if filter.StatusCode != nil {
		clauses = append(clauses, bson.E{Key: "status_code", Value: *filter.StatusCode})
	}

	if filter.From != nil || filter.To != nil {
		rangeQuery := bson.M{}
		if filter.From != nil {
			rangeQuery["$gte"] = filter.From.UTC()
		}
		if filter.To != nil {
			rangeQuery["$lte"] = filter.To.UTC()
		}
		clauses = append(clauses, bson.E{Key: "checked_at", Value: rangeQuery})
	}

	if search := strings.TrimSpace(filter.Search); search != "" {
		pattern := regexp.QuoteMeta(search)
		regex := bson.M{"$regex": pattern, "$options": "i"}
		clauses = append(clauses, bson.E{
			Key: "$or",
			Value: bson.A{
				bson.M{"name": regex},
				bson.M{"url": regex},
				bson.M{"error": regex},
			},
		})
	}

	return clauses
}

func (r *MongoDBCheckLogRepository) Create(ctx context.Context, checkLog *domain.CheckLog) error {
	if r == nil || r.collection == nil || checkLog == nil {
		return domain.ErrInvalidInput
	}

	if checkLog.ID.IsZero() {
		checkLog.ID = bson.NewObjectID()
	}
	if checkLog.CheckedAt.IsZero() {
		checkLog.CheckedAt = time.Now().UTC()
	} else {
		checkLog.CheckedAt = checkLog.CheckedAt.UTC()
	}

	_, err := r.collection.InsertOne(ctx, checkLog)
	return err
}

func (r *MongoDBCheckLogRepository) CreateBatch(ctx context.Context, logs []domain.CheckLog) error {
	if r == nil || r.collection == nil {
		return domain.ErrInvalidInput
	}
	if len(logs) == 0 {
		return nil
	}

	now := time.Now().UTC()
	docs := make([]any, len(logs))
	for i := range logs {
		if logs[i].ID.IsZero() {
			logs[i].ID = bson.NewObjectID()
		}
		if logs[i].CheckedAt.IsZero() {
			logs[i].CheckedAt = now
		} else {
			logs[i].CheckedAt = logs[i].CheckedAt.UTC()
		}
		docs[i] = logs[i]
	}

	_, err := r.collection.InsertMany(ctx, docs)
	return err
}

func (r *MongoDBCheckLogRepository) FindByID(ctx context.Context, id bson.ObjectID) (*domain.CheckLog, error) {
	if r == nil || r.collection == nil {
		return nil, domain.ErrInvalidInput
	}

	var checkLog domain.CheckLog
	err := r.collection.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&checkLog)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}

	return &checkLog, nil
}

func (r *MongoDBCheckLogRepository) FindAll(ctx context.Context, filter CheckLogFilter, limit, offset int64) ([]domain.CheckLog, int64, error) {
	if r == nil || r.collection == nil {
		return nil, 0, domain.ErrInvalidInput
	}

	bsonFilter := buildCheckLogBSONFilter(filter)

	total, err := r.collection.CountDocuments(ctx, bsonFilter)
	if err != nil {
		return nil, 0, err
	}

	findOpts := options.Find().SetSort(bson.D{
		{Key: "checked_at", Value: -1},
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

	logs := make([]domain.CheckLog, 0)
	if err := cursor.All(ctx, &logs); err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

func (r *MongoDBCheckLogRepository) FindByTargetID(ctx context.Context, targetID bson.ObjectID, limit, offset int64) ([]domain.CheckLog, int64, error) {
	return r.FindAll(ctx, CheckLogFilter{TargetID: &targetID}, limit, offset)
}
