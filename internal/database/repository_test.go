package database

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/config"
	"github.com/DaviRodrigues/opspulse/internal/domain"
	"go.mongodb.org/mongo-driver/v2/bson"
)

var (
	_ TargetRepository   = (*MongoDBTargetRepository)(nil)
	_ TargetRepository   = (*MockTargetRepository)(nil)
	_ CheckLogRepository = (*MongoDBCheckLogRepository)(nil)
	_ CheckLogRepository = (*MockCheckLogRepository)(nil)
)

func boolPtr(v bool) *bool           { return &v }
func intPtr(v int) *int              { return &v }
func timePtr(t time.Time) *time.Time { return &t }

func TestMockTargetRepository_CRUDAndFilters(t *testing.T) {
	ctx := context.Background()
	repo := NewMockTargetRepository()

	// Create nil target should fail with ErrInvalidInput
	if err := repo.Create(ctx, nil); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for nil target, got %v", err)
	}

	t1 := &domain.Target{
		Name:           "Auth API",
		URL:            "https://auth.example.com/health",
		Enabled:        true,
		Tags:           []string{"core", "auth"},
		Headers:        map[string]string{"Authorization": "Bearer secret"},
		Body:           map[string]any{"ping": true},
		ExpectedStatus: []int{200, 204},
	}
	if err := repo.Create(ctx, t1); err != nil {
		t.Fatalf("unexpected Create error: %v", err)
	}
	if t1.ID.IsZero() {
		t.Fatal("expected ID to be generated")
	}
	if t1.Method != "GET" {
		t.Errorf("expected default method GET, got %q", t1.Method)
	}
	if t1.Timeout != 5*time.Second {
		t.Errorf("expected default timeout 5s, got %v", t1.Timeout)
	}
	if t1.CreatedAt.IsZero() || t1.UpdatedAt.IsZero() {
		t.Error("expected CreatedAt and UpdatedAt to be set")
	}

	// Mutating caller's slice/map after Create must not leak into repository
	t1.Tags[0] = "mutated"
	t1.Headers["Authorization"] = "mutated"

	found, err := repo.FindByID(ctx, t1.ID)
	if err != nil {
		t.Fatalf("unexpected FindByID error: %v", err)
	}
	if found.Tags[0] != "core" {
		t.Errorf("expected defensive copy on Create, got tag %q", found.Tags[0])
	}
	if found.Headers["Authorization"] != "Bearer secret" {
		t.Errorf("expected defensive copy of Headers, got %q", found.Headers["Authorization"])
	}

	// Mutating returned object from FindByID must not leak into repository
	found.Name = "Mutated Name"
	found.Tags[0] = "mutated-again"
	foundAgain, err := repo.FindByID(ctx, t1.ID)
	if err != nil {
		t.Fatalf("unexpected FindByID error: %v", err)
	}
	if foundAgain.Name != "Auth API" || foundAgain.Tags[0] != "core" {
		t.Errorf("expected repository state isolated from caller mutation, got %+v", foundAgain)
	}

	// Duplicate URL must return ErrDuplicateURL (and wrap ErrAlreadyExists)
	dup := &domain.Target{
		Name:    "Duplicate Auth",
		URL:     "https://auth.example.com/health",
		Enabled: true,
	}
	err = repo.Create(ctx, dup)
	if !errors.Is(err, domain.ErrDuplicateURL) {
		t.Fatalf("expected ErrDuplicateURL, got %v", err)
	}
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("expected ErrDuplicateURL to wrap ErrAlreadyExists, got %v", err)
	}

	// Create second and third targets
	t2 := &domain.Target{
		Name:    "Billing Service",
		URL:     "https://billing.example.com/status",
		Method:  "post",
		Enabled: false,
		Tags:    []string{"payments"},
		Timeout: 10 * time.Second,
	}
	if err := repo.Create(ctx, t2); err != nil {
		t.Fatalf("unexpected Create t2 error: %v", err)
	}
	if t2.Method != "POST" {
		t.Errorf("expected normalized method POST, got %q", t2.Method)
	}

	t3 := &domain.Target{
		Name:    "Orders API",
		URL:     "https://orders.example.com/health",
		Enabled: true,
		Tags:    []string{"core", "orders"},
		Timeout: 3 * time.Second,
	}
	if err := repo.Create(ctx, t3); err != nil {
		t.Fatalf("unexpected Create t3 error: %v", err)
	}

	// FindAll without filter + pagination
	all, total, err := repo.FindAll(ctx, TargetFilter{}, 2, 0)
	if err != nil {
		t.Fatalf("unexpected FindAll error: %v", err)
	}
	if total != 3 || len(all) != 2 {
		t.Fatalf("expected total=3 len=2, got total=%d len=%d", total, len(all))
	}

	page2, total2, err := repo.FindAll(ctx, TargetFilter{}, 2, 2)
	if err != nil {
		t.Fatalf("unexpected FindAll page2 error: %v", err)
	}
	if total2 != 3 || len(page2) != 1 {
		t.Fatalf("expected total=3 len=1, got total=%d len=%d", total2, len(page2))
	}

	// Offset beyond total returns non-nil empty slice
	emptyPage, totalEmpty, err := repo.FindAll(ctx, TargetFilter{}, 10, 99)
	if err != nil {
		t.Fatalf("unexpected FindAll emptyPage error: %v", err)
	}
	if emptyPage == nil || len(emptyPage) != 0 || totalEmpty != 3 {
		t.Fatalf("expected non-nil empty slice with total=3, got %v (total=%d)", emptyPage, totalEmpty)
	}

	// Filter by Enabled
	enabledOnly, enabledTotal, err := repo.FindAll(ctx, TargetFilter{Enabled: boolPtr(true)}, 50, 0)
	if err != nil {
		t.Fatalf("unexpected FindAll enabled error: %v", err)
	}
	if enabledTotal != 2 || len(enabledOnly) != 2 {
		t.Fatalf("expected 2 enabled targets, got total=%d len=%d", enabledTotal, len(enabledOnly))
	}

	// Filter by Tag
	coreTargets, coreTotal, err := repo.FindAll(ctx, TargetFilter{Tag: "core"}, 50, 0)
	if err != nil {
		t.Fatalf("unexpected FindAll tag error: %v", err)
	}
	if coreTotal != 2 || len(coreTargets) != 2 {
		t.Fatalf("expected 2 targets with tag 'core', got total=%d len=%d", coreTotal, len(coreTargets))
	}

	// Filter by Search (case-insensitive on Name or URL)
	searchTargets, searchTotal, err := repo.FindAll(ctx, TargetFilter{Search: "BILLING"}, 50, 0)
	if err != nil {
		t.Fatalf("unexpected FindAll search error: %v", err)
	}
	if searchTotal != 1 || len(searchTargets) != 1 || searchTargets[0].ID != t2.ID {
		t.Fatalf("expected Billing target, got %+v", searchTargets)
	}

	// FindEnabled
	enabledList, err := repo.FindEnabled(ctx)
	if err != nil {
		t.Fatalf("unexpected FindEnabled error: %v", err)
	}
	if len(enabledList) != 2 {
		t.Fatalf("expected 2 enabled targets, got %d", len(enabledList))
	}

	// Update preserving ID and CreatedAt
	originalCreatedAt := t2.CreatedAt
	updatePayload := &domain.Target{
		Name:    "Billing v2",
		URL:     "https://billing.example.com/status",
		Method:  "GET",
		Enabled: true,
		Tags:    []string{"payments", "v2"},
		Timeout: 8 * time.Second,
	}
	if err := repo.Update(ctx, t2.ID, updatePayload); err != nil {
		t.Fatalf("unexpected Update error: %v", err)
	}
	updated, err := repo.FindByID(ctx, t2.ID)
	if err != nil {
		t.Fatalf("unexpected FindByID after update error: %v", err)
	}
	if updated.ID != t2.ID || !updated.CreatedAt.Equal(originalCreatedAt) {
		t.Errorf("expected ID and CreatedAt preserved, got id=%v createdAt=%v", updated.ID, updated.CreatedAt)
	}
	if updated.Name != "Billing v2" || !updated.Enabled {
		t.Errorf("expected updated fields, got %+v", updated)
	}

	// Update colliding with another target's URL must fail with ErrDuplicateURL
	updateCollision := &domain.Target{
		Name: "Billing Collision",
		URL:  "https://auth.example.com/health",
	}
	if err := repo.Update(ctx, t2.ID, updateCollision); !errors.Is(err, domain.ErrDuplicateURL) {
		t.Fatalf("expected ErrDuplicateURL on update collision, got %v", err)
	}

	// Update nonexistent target
	if err := repo.Update(ctx, bson.NewObjectID(), updatePayload); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for missing update target, got %v", err)
	}

	// Delete target
	if err := repo.Delete(ctx, t2.ID); err != nil {
		t.Fatalf("unexpected Delete error: %v", err)
	}
	if _, err := repo.FindByID(ctx, t2.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
	if err := repo.Delete(ctx, t2.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound on second delete, got %v", err)
	}
}

func TestMockCheckLogRepository_CRUDAndFilters(t *testing.T) {
	ctx := context.Background()
	repo := NewMockCheckLogRepository()

	// Empty CreateBatch is a no-op
	if err := repo.CreateBatch(ctx, nil); err != nil {
		t.Fatalf("expected nil error on empty CreateBatch, got %v", err)
	}
	if err := repo.CreateBatch(ctx, []domain.CheckLog{}); err != nil {
		t.Fatalf("expected nil error on zero-length CreateBatch, got %v", err)
	}

	// Create nil log fails
	if err := repo.Create(ctx, nil); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for nil check log, got %v", err)
	}

	targetA := bson.NewObjectID()
	targetB := bson.NewObjectID()
	baseTime := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)

	log1 := &domain.CheckLog{
		TargetID:   targetA,
		Name:       "API A",
		URL:        "https://a.example.com",
		StatusCode: 200,
		Latency:    45 * time.Millisecond,
		IsUp:       true,
		CheckedAt:  baseTime.Add(-2 * time.Minute),
	}
	if err := repo.Create(ctx, log1); err != nil {
		t.Fatalf("unexpected Create error: %v", err)
	}
	if log1.ID.IsZero() {
		t.Fatal("expected CheckLog ID to be generated")
	}

	batch := []domain.CheckLog{
		{
			TargetID:   targetA,
			Name:       "API A",
			URL:        "https://a.example.com",
			StatusCode: 503,
			Latency:    120 * time.Millisecond,
			IsUp:       false,
			Error:      "service unavailable timeout",
			CheckedAt:  baseTime.Add(-1 * time.Minute),
		},
		{
			TargetID:   targetB,
			Name:       "API B",
			URL:        "https://b.example.com",
			StatusCode: 200,
			Latency:    25 * time.Millisecond,
			IsUp:       true,
			CheckedAt:  baseTime,
		},
	}
	if err := repo.CreateBatch(ctx, batch); err != nil {
		t.Fatalf("unexpected CreateBatch error: %v", err)
	}

	// FindByID
	found, err := repo.FindByID(ctx, log1.ID)
	if err != nil {
		t.Fatalf("unexpected FindByID error: %v", err)
	}
	if found.URL != "https://a.example.com" || found.StatusCode != 200 {
		t.Errorf("unexpected FindByID result: %+v", found)
	}
	if _, err := repo.FindByID(ctx, bson.NewObjectID()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// FindAll ordered descending by CheckedAt
	all, total, err := repo.FindAll(ctx, CheckLogFilter{}, 50, 0)
	if err != nil {
		t.Fatalf("unexpected FindAll error: %v", err)
	}
	if total != 3 || len(all) != 3 {
		t.Fatalf("expected 3 logs, got total=%d len=%d", total, len(all))
	}
	if !all[0].CheckedAt.After(all[1].CheckedAt) || !all[1].CheckedAt.After(all[2].CheckedAt) {
		t.Errorf("expected logs sorted descending by CheckedAt, got %v, %v, %v", all[0].CheckedAt, all[1].CheckedAt, all[2].CheckedAt)
	}

	// Filter by IsUp
	downLogs, downTotal, err := repo.FindAll(ctx, CheckLogFilter{IsUp: boolPtr(false)}, 50, 0)
	if err != nil {
		t.Fatalf("unexpected FindAll IsUp=false error: %v", err)
	}
	if downTotal != 1 || len(downLogs) != 1 || downLogs[0].StatusCode != 503 {
		t.Fatalf("expected 1 down log with 503, got %+v", downLogs)
	}

	// Filter by StatusCode
	codeLogs, codeTotal, err := repo.FindAll(ctx, CheckLogFilter{StatusCode: intPtr(200)}, 50, 0)
	if err != nil {
		t.Fatalf("unexpected FindAll StatusCode=200 error: %v", err)
	}
	if codeTotal != 2 || len(codeLogs) != 2 {
		t.Fatalf("expected 2 logs with status 200, got %d", len(codeLogs))
	}

	// Filter by From/To window
	windowLogs, windowTotal, err := repo.FindAll(ctx, CheckLogFilter{
		From: timePtr(baseTime.Add(-90 * time.Second)),
		To:   timePtr(baseTime.Add(-30 * time.Second)),
	}, 50, 0)
	if err != nil {
		t.Fatalf("unexpected FindAll From/To error: %v", err)
	}
	if windowTotal != 1 || len(windowLogs) != 1 || windowLogs[0].StatusCode != 503 {
		t.Fatalf("expected 1 log in window, got %+v", windowLogs)
	}

	// Filter by Search matching Error text
	errSearch, errSearchTotal, err := repo.FindAll(ctx, CheckLogFilter{Search: "TIMEOUT"}, 50, 0)
	if err != nil {
		t.Fatalf("unexpected FindAll Search error: %v", err)
	}
	if errSearchTotal != 1 || len(errSearch) != 1 || errSearch[0].StatusCode != 503 {
		t.Fatalf("expected 1 log matching 'TIMEOUT', got %+v", errSearch)
	}

	// FindByTargetID + pagination
	targetALogs, targetATotal, err := repo.FindByTargetID(ctx, targetA, 1, 0)
	if err != nil {
		t.Fatalf("unexpected FindByTargetID error: %v", err)
	}
	if targetATotal != 2 || len(targetALogs) != 1 || targetALogs[0].StatusCode != 503 {
		t.Fatalf("expected newest targetA log first (503), got total=%d logs=%+v", targetATotal, targetALogs)
	}
}

func TestMockRepositories_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	targetRepo := NewMockTargetRepository()
	logRepo := NewMockCheckLogRepository()

	var wg sync.WaitGroup
	const workers = 16

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			tgt := &domain.Target{
				Name:    fmt.Sprintf("Service-%d", idx),
				URL:     fmt.Sprintf("https://service-%d.example.com", idx),
				Enabled: idx%2 == 0,
				Tags:    []string{"concurrent"},
			}
			if err := targetRepo.Create(ctx, tgt); err != nil {
				t.Errorf("concurrent Target Create failed: %v", err)
				return
			}
			_, _, _ = targetRepo.FindAll(ctx, TargetFilter{Tag: "concurrent"}, 10, 0)
			_, _ = targetRepo.FindEnabled(ctx)
			_, _ = targetRepo.FindByID(ctx, tgt.ID)

			_ = logRepo.Create(ctx, &domain.CheckLog{
				TargetID:   tgt.ID,
				Name:       tgt.Name,
				URL:        tgt.URL,
				StatusCode: 200,
				IsUp:       true,
			})
			_ = logRepo.CreateBatch(ctx, []domain.CheckLog{
				{
					TargetID:   tgt.ID,
					Name:       tgt.Name,
					URL:        tgt.URL,
					StatusCode: 200,
					IsUp:       true,
				},
			})
			_, _, _ = logRepo.FindAll(ctx, CheckLogFilter{TargetID: &tgt.ID}, 10, 0)
		}(i)
	}
	wg.Wait()

	allTargets, totalTargets, err := targetRepo.FindAll(ctx, TargetFilter{}, 50, 0)
	if err != nil {
		t.Fatalf("FindAll targets failed: %v", err)
	}
	if totalTargets != workers || len(allTargets) != workers {
		t.Fatalf("expected %d targets, got total=%d len=%d", workers, totalTargets, len(allTargets))
	}

	allLogs, totalLogs, err := logRepo.FindAll(ctx, CheckLogFilter{}, 100, 0)
	if err != nil {
		t.Fatalf("FindAll logs failed: %v", err)
	}
	if totalLogs != workers*2 || len(allLogs) != workers*2 {
		t.Fatalf("expected %d logs, got total=%d len=%d", workers*2, totalLogs, len(allLogs))
	}
}

func bsonDToMap(d bson.D) map[string]any {
	out := make(map[string]any, len(d))
	for _, e := range d {
		out[e.Key] = e.Value
	}
	return out
}

func TestBSONFilterBuildersAndNilGuards(t *testing.T) {
	// Target filter builder
	tf := buildTargetBSONFilter(TargetFilter{
		Enabled: boolPtr(true),
		Tag:     "prod",
		Search:  "api.example.com?v=1",
	})
	m := bsonDToMap(tf)
	if m["enabled"] != true {
		t.Errorf("expected enabled=true in BSON filter, got %v", m["enabled"])
	}
	if m["tags"] != "prod" {
		t.Errorf("expected tags=prod in BSON filter, got %v", m["tags"])
	}
	orClause, ok := m["$or"].(bson.A)
	if !ok || len(orClause) != 2 {
		t.Fatalf("expected $or clause with 2 elements, got %T %v", m["$or"], m["$or"])
	}

	// CheckLog filter builder
	oid := bson.NewObjectID()
	from := time.Date(2026, 11, 25, 10, 0, 0, 0, time.FixedZone("BRT", -3*3600))
	to := from.Add(2 * time.Hour)
	cf := buildCheckLogBSONFilter(CheckLogFilter{
		TargetID:   &oid,
		IsUp:       boolPtr(false),
		StatusCode: intPtr(500),
		From:       &from,
		To:         &to,
		Search:     "timeout (500)",
	})
	cm := bsonDToMap(cf)
	if cm["target_id"] != oid {
		t.Errorf("expected target_id=%v, got %v", oid, cm["target_id"])
	}
	if cm["is_up"] != false {
		t.Errorf("expected is_up=false, got %v", cm["is_up"])
	}
	if cm["status_code"] != 500 {
		t.Errorf("expected status_code=500, got %v", cm["status_code"])
	}
	checkedAtRange, ok := cm["checked_at"].(bson.M)
	if !ok {
		t.Fatalf("expected checked_at bson.M range, got %T", cm["checked_at"])
	}
	if gte, ok := checkedAtRange["$gte"].(time.Time); !ok || !gte.Equal(from.UTC()) || gte.Location() != time.UTC {
		t.Errorf("expected $gte in UTC, got %v", checkedAtRange["$gte"])
	}
	if lte, ok := checkedAtRange["$lte"].(time.Time); !ok || !lte.Equal(to.UTC()) || lte.Location() != time.UTC {
		t.Errorf("expected $lte in UTC, got %v", checkedAtRange["$lte"])
	}

	// Nil database guards
	if err := EnsureCheckLogsCollection(context.Background(), nil); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for nil db in EnsureCheckLogsCollection, got %v", err)
	}
	if repo := NewMongoDBTargetRepository(nil); repo != nil {
		t.Errorf("expected nil repository for nil db, got %v", repo)
	}
	if repo := NewMongoDBCheckLogRepository(nil); repo != nil {
		t.Errorf("expected nil repository for nil db, got %v", repo)
	}
}

func TestMockRepositories_FindAllLargeLimitNoOverflowPanic(t *testing.T) {
	ctx := context.Background()
	targetRepo := NewMockTargetRepository()
	logRepo := NewMockCheckLogRepository()

	for i := 0; i < 3; i++ {
		tgt := &domain.Target{
			Name:    fmt.Sprintf("Target-%d", i),
			URL:     fmt.Sprintf("https://target-%d.example.com", i),
			Enabled: true,
		}
		if err := targetRepo.Create(ctx, tgt); err != nil {
			t.Fatalf("Create target %d failed: %v", i, err)
		}
		if err := logRepo.Create(ctx, &domain.CheckLog{
			TargetID:   tgt.ID,
			Name:       tgt.Name,
			URL:        tgt.URL,
			StatusCode: 200,
			IsUp:       true,
		}); err != nil {
			t.Fatalf("Create log %d failed: %v", i, err)
		}
	}

	targets, totalTargets, err := targetRepo.FindAll(ctx, TargetFilter{}, math.MaxInt64, 1)
	if err != nil {
		t.Fatalf("Target FindAll with math.MaxInt64 limit failed: %v", err)
	}
	if totalTargets != 3 || len(targets) != 2 {
		t.Fatalf("expected total=3 len=2, got total=%d len=%d", totalTargets, len(targets))
	}

	logs, totalLogs, err := logRepo.FindAll(ctx, CheckLogFilter{}, math.MaxInt64, 1)
	if err != nil {
		t.Fatalf("CheckLog FindAll with math.MaxInt64 limit failed: %v", err)
	}
	if totalLogs != 3 || len(logs) != 2 {
		t.Fatalf("expected total=3 len=2, got total=%d len=%d", totalLogs, len(logs))
	}
}

func TestMockTargetRepository_DeepCloneBodyIsolation(t *testing.T) {
	ctx := context.Background()
	repo := NewMockTargetRepository()

	tgt := &domain.Target{
		Name:    "Deep Clone API",
		URL:     "https://deep.example.com/hook",
		Method:  "POST",
		Enabled: true,
		Body: map[string]any{
			"meta": map[string]any{
				"token": "initial-secret",
			},
			"items": []any{
				"first",
				map[string]any{"nestedKey": "initial-val"},
			},
		},
	}
	if err := repo.Create(ctx, tgt); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Mutate caller's nested structures after Create
	tgt.Body["meta"].(map[string]any)["token"] = "mutated-after-create"
	tgt.Body["items"].([]any)[0] = "mutated-item"
	tgt.Body["items"].([]any)[1].(map[string]any)["nestedKey"] = "mutated-nested"

	verifyClean := func(got *domain.Target, source string) {
		t.Helper()
		meta, ok := got.Body["meta"].(map[string]any)
		if !ok || meta["token"] != "initial-secret" {
			t.Fatalf("%s: expected isolated meta.token='initial-secret', got %#v", source, got.Body["meta"])
		}
		items, ok := got.Body["items"].([]any)
		if !ok || len(items) != 2 || items[0] != "first" {
			t.Fatalf("%s: expected isolated items[0]='first', got %#v", source, got.Body["items"])
		}
		inner, ok := items[1].(map[string]any)
		if !ok || inner["nestedKey"] != "initial-val" {
			t.Fatalf("%s: expected isolated items[1].nestedKey='initial-val', got %#v", source, items[1])
		}
	}

	// FindByID isolation
	byID, err := repo.FindByID(ctx, tgt.ID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	verifyClean(byID, "after Create")
	byID.Body["meta"].(map[string]any)["token"] = "mutated-from-findbyid"
	byID.Body["items"].([]any)[1].(map[string]any)["nestedKey"] = "mutated-from-findbyid"

	// FindAll isolation
	all, _, err := repo.FindAll(ctx, TargetFilter{}, 10, 0)
	if err != nil || len(all) != 1 {
		t.Fatalf("FindAll failed: err=%v len=%d", err, len(all))
	}
	verifyClean(all[0], "FindAll after FindByID mutation")
	all[0].Body["items"].([]any)[0] = "mutated-from-findall"

	// FindEnabled isolation
	enabled, err := repo.FindEnabled(ctx)
	if err != nil || len(enabled) != 1 {
		t.Fatalf("FindEnabled failed: err=%v len=%d", err, len(enabled))
	}
	verifyClean(enabled[0], "FindEnabled after FindAll mutation")

	// Update isolation
	updateReq := &domain.Target{
		Name:    "Deep Clone API v2",
		URL:     "https://deep.example.com/hook",
		Method:  "POST",
		Enabled: true,
		Body: map[string]any{
			"meta":  map[string]any{"token": "updated-secret"},
			"items": []any{"updated-first"},
		},
	}
	if err := repo.Update(ctx, tgt.ID, updateReq); err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	updateReq.Body["meta"].(map[string]any)["token"] = "mutated-after-update"
	updateReq.Body["items"].([]any)[0] = "mutated-after-update"

	afterUpdate, err := repo.FindByID(ctx, tgt.ID)
	if err != nil {
		t.Fatalf("FindByID after Update failed: %v", err)
	}
	if afterUpdate.Body["meta"].(map[string]any)["token"] != "updated-secret" {
		t.Fatalf("expected updated-secret, got %#v", afterUpdate.Body["meta"])
	}
	if afterUpdate.Body["items"].([]any)[0] != "updated-first" {
		t.Fatalf("expected updated-first, got %#v", afterUpdate.Body["items"])
	}

	// Concurrent reads + caller mutations under -race
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			got, err := repo.FindByID(ctx, tgt.ID)
			if err != nil {
				t.Errorf("concurrent FindByID failed: %v", err)
				return
			}
			got.Body["meta"].(map[string]any)["token"] = fmt.Sprintf("race-%d", idx)
			got.Body["items"].([]any)[0] = fmt.Sprintf("race-item-%d", idx)

			list, _, err := repo.FindAll(ctx, TargetFilter{}, 10, 0)
			if err == nil && len(list) > 0 {
				list[0].Body["meta"].(map[string]any)["extra"] = idx
			}

			en, err := repo.FindEnabled(ctx)
			if err == nil && len(en) > 0 {
				en[0].Body["items"].([]any)[0] = "race-enabled"
			}
		}(i)
	}
	wg.Wait()
}

func TestMockTargetRepository_OrderingMatchesMongoDB(t *testing.T) {
	ctx := context.Background()
	repo := NewMockTargetRepository()

	tOldest := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	tMiddle := time.Date(2026, 11, 25, 11, 0, 0, 0, time.UTC)
	tNewest := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)

	idLow, err := bson.ObjectIDFromHex("650000000000000000000001")
	if err != nil {
		t.Fatalf("invalid hex idLow: %v", err)
	}
	idHigh, err := bson.ObjectIDFromHex("650000000000000000000009")
	if err != nil {
		t.Fatalf("invalid hex idHigh: %v", err)
	}

	// Insert intentionally out of CreatedAt and ID order:
	// 1) middle timestamp, lower ID
	// 2) newest timestamp
	// 3) oldest timestamp
	// 4) middle timestamp, higher ID (tie-breaker must place idHigh before idLow)
	inputs := []*domain.Target{
		{ID: idLow, Name: "Middle-Low", URL: "https://mid-low.example.com", Enabled: true, CreatedAt: tMiddle},
		{Name: "Newest", URL: "https://newest.example.com", Enabled: true, CreatedAt: tNewest},
		{Name: "Oldest", URL: "https://oldest.example.com", Enabled: true, CreatedAt: tOldest},
		{ID: idHigh, Name: "Middle-High", URL: "https://mid-high.example.com", Enabled: true, CreatedAt: tMiddle},
	}
	for _, in := range inputs {
		if err := repo.Create(ctx, in); err != nil {
			t.Fatalf("Create %s failed: %v", in.Name, err)
		}
	}

	expectedOrder := []string{"Newest", "Middle-High", "Middle-Low", "Oldest"}

	all, total, err := repo.FindAll(ctx, TargetFilter{}, 50, 0)
	if err != nil {
		t.Fatalf("FindAll failed: %v", err)
	}
	if total != 4 || len(all) != 4 {
		t.Fatalf("expected 4 targets, got total=%d len=%d", total, len(all))
	}
	for i, want := range expectedOrder {
		if all[i].Name != want {
			t.Errorf("FindAll[%d]: expected %s, got %s", i, want, all[i].Name)
		}
	}

	enabled, err := repo.FindEnabled(ctx)
	if err != nil {
		t.Fatalf("FindEnabled failed: %v", err)
	}
	if len(enabled) != 4 {
		t.Fatalf("expected 4 enabled targets, got %d", len(enabled))
	}
	for i, want := range expectedOrder {
		if enabled[i].Name != want {
			t.Errorf("FindEnabled[%d]: expected %s, got %s", i, want, enabled[i].Name)
		}
	}
}

func TestMockTargetRepository_CreateDuplicateAssignedID(t *testing.T) {
	ctx := context.Background()
	repo := NewMockTargetRepository()

	sharedID := bson.NewObjectID()
	first := &domain.Target{
		ID:      sharedID,
		Name:    "Original Target",
		URL:     "https://original.example.com",
		Enabled: true,
	}
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("Create first failed: %v", err)
	}

	second := &domain.Target{
		ID:      sharedID,
		Name:    "Colliding ID Target",
		URL:     "https://different-url.example.com",
		Enabled: true,
	}
	err := repo.Create(ctx, second)
	if !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists for duplicate pre-assigned ID, got %v", err)
	}

	got, err := repo.FindByID(ctx, sharedID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if got.Name != "Original Target" || got.URL != "https://original.example.com" {
		t.Fatalf("expected original target to remain unmodified, got %+v", got)
	}

	all, total, err := repo.FindAll(ctx, TargetFilter{}, 50, 0)
	if err != nil {
		t.Fatalf("FindAll failed: %v", err)
	}
	if total != 1 || len(all) != 1 {
		t.Fatalf("expected exactly 1 target (no duplicate order entry), got total=%d len=%d", total, len(all))
	}
}

func TestNewMongoClient_TimeoutFallbackAndDisconnectOnPingFailure(t *testing.T) {
	t.Run("Timeout <= 0 falls back to 5s instead of immediate deadline exceeded", func(t *testing.T) {
		for _, timeoutVal := range []time.Duration{0, -2 * time.Second} {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("net.Listen failed: %v", err)
			}
			defer ln.Close()

			// Accept connections and hold them open silently so Ping waits for parent ctx deadline
			var connsMu sync.Mutex
			var conns []net.Conn
			defer func() {
				connsMu.Lock()
				for _, c := range conns {
					_ = c.Close()
				}
				connsMu.Unlock()
			}()
			go func() {
				for {
					c, acceptErr := ln.Accept()
					if acceptErr != nil {
						return
					}
					connsMu.Lock()
					conns = append(conns, c)
					connsMu.Unlock()
				}
			}()

			parentCtx, cancel := context.WithTimeout(context.Background(), 140*time.Millisecond)
			defer cancel()

			start := time.Now()
			client, err := NewMongoClient(parentCtx, config.DatabaseConfig{
				URI:     "mongodb://" + ln.Addr().String() + "/?directConnection=true",
				Name:    "opspulse",
				Timeout: timeoutVal,
			})
			elapsed := time.Since(start)

			if err == nil {
				_ = client.Close(context.Background())
				t.Fatalf("expected Ping error for unresponsive TCP listener (timeout=%v)", timeoutVal)
			}
			if client != nil {
				t.Fatalf("expected nil MongoClient on failure, got %+v", client)
			}
			// If Timeout <= 0 was not guarded, context.WithTimeout(ctx, timeoutVal) would expire immediately (< 20ms).
			if elapsed < 100*time.Millisecond {
				t.Fatalf("expected Timeout=%v to fall back to 5s (bounded by 140ms parent ctx), but failed in %v: %v", timeoutVal, elapsed, err)
			}
		}
	})

	t.Run("Ping failure disconnects client and closes socket", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("net.Listen failed: %v", err)
		}
		defer ln.Close()

		connClosedCh := make(chan error, 4)
		go func() {
			for {
				c, acceptErr := ln.Accept()
				if acceptErr != nil {
					return
				}
				go func(conn net.Conn) {
					defer conn.Close()
					buf := make([]byte, 1024)
					for {
						if _, readErr := conn.Read(buf); readErr != nil {
							connClosedCh <- readErr
							return
						}
					}
				}(c)
			}
		}()

		client, err := NewMongoClient(context.Background(), config.DatabaseConfig{
			URI:     "mongodb://" + ln.Addr().String() + "/?directConnection=true",
			Name:    "opspulse",
			Timeout: 120 * time.Millisecond,
		})
		if err == nil {
			_ = client.Close(context.Background())
			t.Fatal("expected Ping failure against silent TCP listener")
		}

		// When Disconnect is called on Ping failure, the driver closes its TCP connection(s) promptly.
		select {
		case readErr := <-connClosedCh:
			if !errors.Is(readErr, io.EOF) && !errors.Is(readErr, net.ErrClosed) {
				t.Logf("listener observed socket close: %v", readErr)
			}
		case <-time.After(1 * time.Second):
			t.Fatal("expected client.Disconnect to close TCP connection on Ping failure, but socket remained open")
		}
	})

	t.Run("Nil MongoClient Close and Database guards", func(t *testing.T) {
		var nilClient *MongoClient
		if nilClient.Database() != nil {
			t.Fatal("expected nil Database() for nil MongoClient")
		}
		if err := nilClient.Close(context.Background()); err != nil {
			t.Fatalf("expected nil Close() error for nil MongoClient, got %v", err)
		}
	})
}
