//go:build e2e

package mongofake_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/DaviRodrigues/opspulse/tests/e2e/mongofake"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type testTargetDoc struct {
	ID        bson.ObjectID `bson:"_id,omitempty"`
	Name      string        `bson:"name"`
	URL       string        `bson:"url"`
	Enabled   bool          `bson:"enabled"`
	Code      int           `bson:"code"`
	Tags      []string      `bson:"tags"`
	CreatedAt time.Time     `bson:"created_at"`
}

func setupClient(t *testing.T) (*mongofake.Server, *mongo.Client, *mongo.Database) {
	t.Helper()
	srv, err := mongofake.NewServer()
	if err != nil {
		t.Fatalf("NewServer() error: %v", err)
	}
	t.Cleanup(func() {
		_ = srv.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(srv.URI()))
	if err != nil {
		t.Fatalf("mongo.Connect() error: %v", err)
	}
	t.Cleanup(func() {
		dCtx, dCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer dCancel()
		_ = client.Disconnect(dCtx)
	})

	if err := client.Ping(ctx, nil); err != nil {
		t.Fatalf("client.Ping() error: %v", err)
	}

	return srv, client, client.Database("opspulse_test")
}

func TestMongoFake_PingAndCollections(t *testing.T) {
	srv, _, db := setupClient(t)
	ctx := context.Background()

	tsOpts := options.TimeSeries().SetTimeField("checked_at").SetMetaField("target_id").SetGranularity("seconds")
	createOpts := options.CreateCollection().SetTimeSeriesOptions(tsOpts).SetExpireAfterSeconds(2592000)
	if err := db.CreateCollection(ctx, "check_logs", createOpts); err != nil {
		t.Fatalf("CreateCollection() error: %v", err)
	}

	names, err := db.ListCollectionNames(ctx, bson.M{"name": "check_logs"})
	if err != nil {
		t.Fatalf("ListCollectionNames() error: %v", err)
	}
	if len(names) != 1 || names[0] != "check_logs" {
		t.Fatalf("expected [check_logs], got %v", names)
	}
	if got := srv.Count("opspulse_test", "check_logs"); got != 0 {
		t.Fatalf("expected 0 docs, got %d", got)
	}
}

func TestMongoFake_InsertUniqueIndexAndDuplicateKey(t *testing.T) {
	srv, _, db := setupClient(t)
	ctx := context.Background()
	coll := db.Collection("targets")

	_, err := coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "url", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("idx_targets_url_unique"),
	})
	if err != nil {
		t.Fatalf("CreateOne index error: %v", err)
	}

	id1 := bson.NewObjectID()
	now := time.Now().UTC().Truncate(time.Millisecond)
	res, err := coll.InsertOne(ctx, testTargetDoc{
		ID:        id1,
		Name:      "Primary API",
		URL:       "https://api.example.com/health",
		Enabled:   true,
		Code:      200,
		Tags:      []string{"prod", "core"},
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("InsertOne() error: %v", err)
	}
	if res.InsertedID != id1 {
		t.Fatalf("expected InsertedID %v, got %v", id1, res.InsertedID)
	}

	// Duplicate URL must return E11000 recognized by mongo.IsDuplicateKeyError
	_, err = coll.InsertOne(ctx, testTargetDoc{
		ID:        bson.NewObjectID(),
		Name:      "Duplicate API",
		URL:       "https://api.example.com/health",
		Enabled:   true,
		Code:      200,
		CreatedAt: now,
	})
	if err == nil {
		t.Fatal("expected duplicate key error, got nil")
	}
	if !mongo.IsDuplicateKeyError(err) {
		t.Fatalf("expected mongo.IsDuplicateKeyError(err)==true, got: %v", err)
	}

	// InsertMany batch (OP_MSG section kind 1)
	many := []any{
		testTargetDoc{ID: bson.NewObjectID(), Name: "Auth Service", URL: "https://auth.example.com", Enabled: true, Code: 200, Tags: []string{"prod"}, CreatedAt: now.Add(1 * time.Second)},
		testTargetDoc{ID: bson.NewObjectID(), Name: "Billing Worker", URL: "https://billing.example.com", Enabled: false, Code: 503, Tags: []string{"staging"}, CreatedAt: now.Add(2 * time.Second)},
		testTargetDoc{ID: bson.NewObjectID(), Name: "Search Engine", URL: "https://search.example.com", Enabled: true, Code: 204, Tags: []string{"prod", "search"}, CreatedAt: now.Add(3 * time.Second)},
	}
	manyRes, err := coll.InsertMany(ctx, many)
	if err != nil {
		t.Fatalf("InsertMany() error: %v", err)
	}
	if len(manyRes.InsertedIDs) != 3 {
		t.Fatalf("expected 3 InsertedIDs, got %d", len(manyRes.InsertedIDs))
	}
	if got := srv.Count("opspulse_test", "targets"); got != 4 {
		t.Fatalf("expected 4 docs, got %d", got)
	}

	srv.Clear("opspulse_test", "targets")
	if got := srv.Count("opspulse_test", "targets"); got != 0 {
		t.Fatalf("expected 0 docs after Clear, got %d", got)
	}
}

func TestMongoFake_FindOperatorsSortSkipLimitAndCursor(t *testing.T) {
	_, _, db := setupClient(t)
	ctx := context.Background()
	coll := db.Collection("check_logs")

	base := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	docs := []any{
		bson.D{{Key: "name", Value: "Alpha API"}, {Key: "status_code", Value: 200}, {Key: "is_up", Value: true}, {Key: "tags", Value: bson.A{"prod", "web"}}, {Key: "checked_at", Value: base}},
		bson.D{{Key: "name", Value: "Beta Worker"}, {Key: "status_code", Value: 500}, {Key: "is_up", Value: false}, {Key: "tags", Value: bson.A{"prod", "async"}}, {Key: "checked_at", Value: base.Add(10 * time.Second)}},
		bson.D{{Key: "name", Value: "Gamma API"}, {Key: "status_code", Value: 201}, {Key: "is_up", Value: true}, {Key: "tags", Value: bson.A{"staging"}}, {Key: "checked_at", Value: base.Add(20 * time.Second)}},
		bson.D{{Key: "name", Value: "Delta Cache"}, {Key: "status_code", Value: 503}, {Key: "is_up", Value: false}, {Key: "tags", Value: bson.A{"prod"}}, {Key: "checked_at", Value: base.Add(30 * time.Second)}},
	}
	if _, err := coll.InsertMany(ctx, docs); err != nil {
		t.Fatalf("InsertMany() error: %v", err)
	}

	// Range query with $gte / $lte on time.Time, plus $regex with case-insensitive option
	filter := bson.M{
		"checked_at": bson.M{
			"$gte": base.Add(5 * time.Second),
			"$lte": base.Add(30 * time.Second),
		},
		"$or": []bson.M{
			{"name": bson.M{"$regex": "api", "$options": "i"}},
			{"status_code": bson.M{"$in": []int{500, 503}}},
		},
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "checked_at", Value: -1}}).
		SetSkip(1).
		SetLimit(2).
		SetBatchSize(1) // exercises getMore cursor!

	cur, err := coll.Find(ctx, filter, opts)
	if err != nil {
		t.Fatalf("Find() error: %v", err)
	}
	var out []bson.M
	if err := cur.All(ctx, &out); err != nil {
		t.Fatalf("cur.All() error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 docs after skip=1 limit=2, got %d (%v)", len(out), out)
	}
	if out[0]["name"] != "Gamma API" || out[1]["name"] != "Beta Worker" {
		t.Fatalf("unexpected sort/skip/limit order: %v", out)
	}

	// FindOne with array element equality
	var one bson.M
	if err := coll.FindOne(ctx, bson.M{"tags": "async"}).Decode(&one); err != nil {
		t.Fatalf("FindOne() error: %v", err)
	}
	if one["name"] != "Beta Worker" {
		t.Fatalf("expected Beta Worker, got %v", one["name"])
	}

	// CountDocuments (uses aggregate $match + $group)
	cnt, err := coll.CountDocuments(ctx, bson.M{"is_up": false})
	if err != nil {
		t.Fatalf("CountDocuments() error: %v", err)
	}
	if cnt != 2 {
		t.Fatalf("expected CountDocuments=2, got %d", cnt)
	}

	// CountDocuments returning 0
	zeroCnt, err := coll.CountDocuments(ctx, bson.M{"status_code": 999})
	if err != nil {
		t.Fatalf("CountDocuments(0) error: %v", err)
	}
	if zeroCnt != 0 {
		t.Fatalf("expected CountDocuments=0, got %d", zeroCnt)
	}
}

func TestMongoFake_UpdateFindOneAndModifyDeleteAndAggregate(t *testing.T) {
	_, _, db := setupClient(t)
	ctx := context.Background()
	coll := db.Collection("targets")

	_, _ = coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "url", Value: 1}},
		Options: options.Index().SetUnique(true),
	})

	id1 := bson.NewObjectID()
	id2 := bson.NewObjectID()
	_, err := coll.InsertMany(ctx, []any{
		bson.D{{Key: "_id", Value: id1}, {Key: "name", Value: "Svc1"}, {Key: "url", Value: "https://svc1.test"}, {Key: "hits", Value: int32(1)}, {Key: "note", Value: "temp"}},
		bson.D{{Key: "_id", Value: id2}, {Key: "name", Value: "Svc2"}, {Key: "url", Value: "https://svc2.test"}, {Key: "hits", Value: int32(5)}},
	})
	if err != nil {
		t.Fatalf("InsertMany() error: %v", err)
	}

	// UpdateOne with $set, $inc, $unset
	updRes, err := coll.UpdateOne(ctx,
		bson.M{"_id": id1},
		bson.M{
			"$set":   bson.M{"name": "Svc1-Updated"},
			"$inc":   bson.M{"hits": 3},
			"$unset": bson.M{"note": ""},
		},
	)
	if err != nil {
		t.Fatalf("UpdateOne() error: %v", err)
	}
	if updRes.MatchedCount != 1 || updRes.ModifiedCount != 1 {
		t.Fatalf("unexpected UpdateOne result: %+v", updRes)
	}

	// UpdateOne duplicate URL collision must fail with duplicate key error
	_, err = coll.UpdateOne(ctx,
		bson.M{"_id": id1},
		bson.M{"$set": bson.M{"url": "https://svc2.test"}},
	)
	if !mongo.IsDuplicateKeyError(err) {
		t.Fatalf("expected duplicate key error on UpdateOne, got %v", err)
	}

	// FindOneAndUpdate returning updated document
	var after bson.M
	err = coll.FindOneAndUpdate(ctx,
		bson.M{"_id": id1},
		bson.M{"$set": bson.M{"name": "Svc1-Final"}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&after)
	if err != nil {
		t.Fatalf("FindOneAndUpdate() error: %v", err)
	}
	if after["name"] != "Svc1-Final" {
		t.Fatalf("expected Svc1-Final, got %v", after["name"])
	}
	if _, hasNote := after["note"]; hasNote {
		t.Fatal("expected $unset note to remove field")
	}
	if hits, ok := after["hits"].(int32); !ok || hits != 4 {
		t.Fatalf("expected hits=4 (int32), got %T(%v)", after["hits"], after["hits"])
	}

	// Aggregate with $match + $count
	aggCur, err := coll.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"hits": bson.M{"$gte": 4}}}},
		{{Key: "$count", Value: "total"}},
	})
	if err != nil {
		t.Fatalf("Aggregate() error: %v", err)
	}
	var aggOut []bson.M
	if err := aggCur.All(ctx, &aggOut); err != nil {
		t.Fatalf("aggCur.All() error: %v", err)
	}
	if len(aggOut) != 1 || aggOut[0]["total"] != int32(2) {
		t.Fatalf("expected [{total: 2}], got %v", aggOut)
	}

	// FindOneAndDelete
	var deletedDoc bson.M
	if err := coll.FindOneAndDelete(ctx, bson.M{"_id": id2}).Decode(&deletedDoc); err != nil {
		t.Fatalf("FindOneAndDelete() error: %v", err)
	}
	if deletedDoc["name"] != "Svc2" {
		t.Fatalf("expected Svc2 deleted, got %v", deletedDoc["name"])
	}

	// DeleteOne
	delRes, err := coll.DeleteOne(ctx, bson.M{"_id": id1})
	if err != nil {
		t.Fatalf("DeleteOne() error: %v", err)
	}
	if delRes.DeletedCount != 1 {
		t.Fatalf("expected DeletedCount=1, got %d", delRes.DeletedCount)
	}

	// FindOne after delete returns ErrNoDocuments
	err = coll.FindOne(ctx, bson.M{"_id": id1}).Err()
	if err != mongo.ErrNoDocuments {
		t.Fatalf("expected ErrNoDocuments, got %v", err)
	}
}

func TestMongoFake_ConcurrentAccess(t *testing.T) {
	srv, _, db := setupClient(t)
	ctx := context.Background()
	coll := db.Collection("concurrent")

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				id := bson.NewObjectID()
				if _, err := coll.InsertOne(ctx, bson.M{"_id": id, "worker": worker, "seq": j}); err != nil {
					t.Errorf("InsertOne error: %v", err)
					return
				}
				var out bson.M
				if err := coll.FindOne(ctx, bson.M{"_id": id}).Decode(&out); err != nil {
					t.Errorf("FindOne error: %v", err)
					return
				}
			}
		}(i)
	}
	wg.Wait()

	if got := srv.Count("opspulse_test", "concurrent"); got != 80 {
		t.Fatalf("expected 80 docs, got %d", got)
	}
}

func TestMongoFake_AggregateGroupSumFloatAndNumeric(t *testing.T) {
	_, _, db := setupClient(t)
	ctx := context.Background()
	coll := db.Collection("metrics")

	_, err := coll.InsertMany(ctx, []any{
		bson.D{{Key: "grp", Value: "floats"}, {Key: "val", Value: 1.5}},
		bson.D{{Key: "grp", Value: "floats"}, {Key: "val", Value: 2.5}},
		bson.D{{Key: "grp", Value: "mixed"}, {Key: "val", Value: int32(2)}},
		bson.D{{Key: "grp", Value: "mixed"}, {Key: "val", Value: 1.5}},
		bson.D{{Key: "grp", Value: "ints"}, {Key: "val", Value: int32(2)}},
		bson.D{{Key: "grp", Value: "ints"}, {Key: "val", Value: int32(3)}},
	})
	if err != nil {
		t.Fatalf("InsertMany() error: %v", err)
	}

	cur, err := coll.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$grp"},
			{Key: "sum", Value: bson.D{{Key: "$sum", Value: "$val"}}},
			{Key: "avg", Value: bson.D{{Key: "$avg", Value: "$val"}}},
			{Key: "min", Value: bson.D{{Key: "$min", Value: "$val"}}},
			{Key: "max", Value: bson.D{{Key: "$max", Value: "$val"}}},
		}}},
	})
	if err != nil {
		t.Fatalf("Aggregate() error: %v", err)
	}
	var out []bson.M
	if err := cur.All(ctx, &out); err != nil {
		t.Fatalf("cur.All() error: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("expected 3 groups, got %d: %v", len(out), out)
	}

	byGrp := make(map[string]bson.M, len(out))
	for _, doc := range out {
		byGrp[doc["_id"].(string)] = doc
	}

	floats := byGrp["floats"]
	if s, ok := floats["sum"].(float64); !ok || s != 4.0 {
		t.Fatalf("expected floats sum=float64(4.0), got %T(%v)", floats["sum"], floats["sum"])
	}
	if avg, ok := floats["avg"].(float64); !ok || avg != 2.0 {
		t.Fatalf("expected floats avg=2.0, got %T(%v)", floats["avg"], floats["avg"])
	}
	if mn, ok := floats["min"].(float64); !ok || mn != 1.5 {
		t.Fatalf("expected floats min=1.5, got %T(%v)", floats["min"], floats["min"])
	}
	if mx, ok := floats["max"].(float64); !ok || mx != 2.5 {
		t.Fatalf("expected floats max=2.5, got %T(%v)", floats["max"], floats["max"])
	}

	mixed := byGrp["mixed"]
	if s, ok := mixed["sum"].(float64); !ok || s != 3.5 {
		t.Fatalf("expected mixed sum=float64(3.5), got %T(%v)", mixed["sum"], mixed["sum"])
	}

	ints := byGrp["ints"]
	if s, ok := ints["sum"].(int64); !ok || s != 5 {
		t.Fatalf("expected ints sum=int64(5), got %T(%v)", ints["sum"], ints["sum"])
	}
}

func TestMongoFake_ListIndexes(t *testing.T) {
	_, _, db := setupClient(t)
	ctx := context.Background()
	coll := db.Collection("targets_idx")

	// Non-existent collection should return empty index list without error
	emptySpecs, err := coll.Indexes().ListSpecifications(ctx)
	if err != nil {
		t.Fatalf("ListSpecifications on empty coll error: %v", err)
	}
	if len(emptySpecs) != 0 {
		t.Fatalf("expected 0 index specs on non-existent collection, got %d", len(emptySpecs))
	}

	if err := db.CreateCollection(ctx, "targets_idx"); err != nil {
		t.Fatalf("CreateCollection() error: %v", err)
	}
	_, err = coll.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "url", Value: int32(1)}},
			Options: options.Index().SetName("idx_url_unique").SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "enabled", Value: int32(1)}, {Key: "created_at", Value: int32(-1)}},
			Options: options.Index().SetName("idx_enabled_created"),
		},
	})
	if err != nil {
		t.Fatalf("CreateMany() error: %v", err)
	}

	specs, err := coll.Indexes().ListSpecifications(ctx)
	if err != nil {
		t.Fatalf("ListSpecifications() error: %v", err)
	}
	if len(specs) != 3 {
		t.Fatalf("expected 3 indexes (_id_ + 2 created), got %d: %+v", len(specs), specs)
	}

	byName := make(map[string]mongo.IndexSpecification, len(specs))
	for _, sp := range specs {
		byName[sp.Name] = sp
	}
	if _, ok := byName["_id_"]; !ok {
		t.Fatalf("expected default _id_ index in ListSpecifications, got %+v", byName)
	}
	urlIdx, ok := byName["idx_url_unique"]
	if !ok {
		t.Fatalf("expected idx_url_unique in ListSpecifications, got %+v", byName)
	}
	if urlIdx.Unique == nil || !*urlIdx.Unique {
		t.Fatalf("expected idx_url_unique Unique=true, got %v", urlIdx.Unique)
	}
	if _, ok := byName["idx_enabled_created"]; !ok {
		t.Fatalf("expected idx_enabled_created in ListSpecifications, got %+v", byName)
	}
}

func TestMongoFake_DuplicateCreateCollectionError(t *testing.T) {
	_, _, db := setupClient(t)
	ctx := context.Background()

	if err := db.CreateCollection(ctx, "dup_coll"); err != nil {
		t.Fatalf("first CreateCollection() error: %v", err)
	}
	err := db.CreateCollection(ctx, "dup_coll")
	if err == nil {
		t.Fatal("expected NamespaceExists error on duplicate CreateCollection, got nil")
	}
	var cmdErr mongo.CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("expected mongo.CommandError, got %T: %v", err, err)
	}
	if cmdErr.Code != 48 || cmdErr.Name != "NamespaceExists" {
		t.Fatalf("expected code 48 (NamespaceExists), got code=%d name=%q msg=%q", cmdErr.Code, cmdErr.Name, cmdErr.Message)
	}
}

func TestMongoFake_InvalidRegexError(t *testing.T) {
	_, _, db := setupClient(t)
	ctx := context.Background()
	coll := db.Collection("regex_test")

	if _, err := coll.InsertOne(ctx, bson.D{{Key: "name", Value: "Alpha"}}); err != nil {
		t.Fatalf("InsertOne() error: %v", err)
	}

	assertInvalidRegexErr := func(t *testing.T, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("expected invalid regex error, got nil")
		}
		var cmdErr mongo.CommandError
		if !errors.As(err, &cmdErr) {
			t.Fatalf("expected mongo.CommandError, got %T: %v", err, err)
		}
		if cmdErr.Code != 51091 || cmdErr.Name != "BadValue" {
			t.Fatalf("expected code 51091 (BadValue), got code=%d name=%q msg=%q", cmdErr.Code, cmdErr.Name, cmdErr.Message)
		}
	}

	t.Run("find_operator_regex", func(t *testing.T) {
		_, err := coll.Find(ctx, bson.M{"name": bson.M{"$regex": "[unclosed"}})
		assertInvalidRegexErr(t, err)
	})

	t.Run("find_bson_regex", func(t *testing.T) {
		_, err := coll.Find(ctx, bson.M{"name": bson.Regex{Pattern: "[unclosed"}})
		assertInvalidRegexErr(t, err)
	})

	t.Run("count_documents_invalid_regex", func(t *testing.T) {
		_, err := coll.CountDocuments(ctx, bson.M{"$or": []bson.M{
			{"name": "Alpha"},
			{"name": bson.M{"$regex": "(?P<bad"}},
		}})
		assertInvalidRegexErr(t, err)
	})
}
