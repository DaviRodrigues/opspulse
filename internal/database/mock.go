package database

import (
	"context"
	"maps"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/domain"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func cloneAny(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return cloneMap(val)
	case []any:
		return cloneSlice(val)
	case map[string]string:
		return maps.Clone(val)
	case []string:
		return slices.Clone(val)
	case []int:
		return slices.Clone(val)
	case []byte:
		return slices.Clone(val)
	case bson.M:
		if val == nil {
			return nil
		}
		out := make(bson.M, len(val))
		for k, item := range val {
			out[k] = cloneAny(item)
		}
		return out
	case bson.A:
		if val == nil {
			return nil
		}
		out := make(bson.A, len(val))
		for i, item := range val {
			out[i] = cloneAny(item)
		}
		return out
	case bson.D:
		if val == nil {
			return nil
		}
		out := make(bson.D, len(val))
		for i, elem := range val {
			out[i] = bson.E{Key: elem.Key, Value: cloneAny(elem.Value)}
		}
		return out
	default:
		return v
	}
}

func cloneMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = cloneAny(v)
	}
	return dst
}

func cloneSlice(src []any) []any {
	if src == nil {
		return nil
	}
	dst := make([]any, len(src))
	for i, v := range src {
		dst[i] = cloneAny(v)
	}
	return dst
}

func cloneTarget(src *domain.Target) *domain.Target {
	if src == nil {
		return nil
	}

	dst := *src
	if src.Tags != nil {
		dst.Tags = slices.Clone(src.Tags)
	}
	if src.Headers != nil {
		dst.Headers = maps.Clone(src.Headers)
	}
	if src.ExpectedStatus != nil {
		dst.ExpectedStatus = slices.Clone(src.ExpectedStatus)
	}
	if src.Body != nil {
		dst.Body = cloneMap(src.Body)
	}

	return &dst
}

type MockTargetRepository struct {
	mu      sync.RWMutex
	targets map[string]*domain.Target
	order   []string
}

func NewMockTargetRepository() *MockTargetRepository {
	return &MockTargetRepository{
		targets: make(map[string]*domain.Target),
		order:   make([]string, 0),
	}
}

func (m *MockTargetRepository) Create(ctx context.Context, target *domain.Target) error {
	if target == nil {
		return domain.ErrInvalidInput
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if !target.ID.IsZero() {
		if _, exists := m.targets[target.ID.Hex()]; exists {
			return domain.ErrAlreadyExists
		}
	}

	for _, existing := range m.targets {
		if existing.URL == target.URL {
			return domain.ErrDuplicateURL
		}
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

	idStr := target.ID.Hex()
	m.targets[idStr] = cloneTarget(target)
	m.order = append(m.order, idStr)

	return nil
}

func (m *MockTargetRepository) FindByID(ctx context.Context, id bson.ObjectID) (*domain.Target, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	target, exists := m.targets[id.Hex()]
	if !exists {
		return nil, domain.ErrNotFound
	}

	return cloneTarget(target), nil
}

func matchesTargetFilter(t *domain.Target, filter TargetFilter) bool {
	if filter.Enabled != nil && t.Enabled != *filter.Enabled {
		return false
	}
	if tag := strings.TrimSpace(filter.Tag); tag != "" && !slices.Contains(t.Tags, tag) {
		return false
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		lowerSearch := strings.ToLower(search)
		if !strings.Contains(strings.ToLower(t.Name), lowerSearch) &&
			!strings.Contains(strings.ToLower(t.URL), lowerSearch) {
			return false
		}
	}
	return true
}

func (m *MockTargetRepository) FindAll(ctx context.Context, filter TargetFilter, limit, offset int64) ([]*domain.Target, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	matched := make([]*domain.Target, 0, len(m.order))
	for i := len(m.order) - 1; i >= 0; i-- {
		t, exists := m.targets[m.order[i]]
		if !exists {
			continue
		}
		if matchesTargetFilter(t, filter) {
			matched = append(matched, cloneTarget(t))
		}
	}

	sort.SliceStable(matched, func(i, j int) bool {
		if matched[i].CreatedAt.Equal(matched[j].CreatedAt) {
			return matched[i].ID.Hex() > matched[j].ID.Hex()
		}
		return matched[i].CreatedAt.After(matched[j].CreatedAt)
	})

	total := int64(len(matched))
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		return []*domain.Target{}, total, nil
	}

	end := total
	if limit > 0 && limit < total-offset {
		end = offset + limit
	}

	result := make([]*domain.Target, end-offset)
	copy(result, matched[offset:end])
	return result, total, nil
}

func (m *MockTargetRepository) FindEnabled(ctx context.Context) ([]*domain.Target, error) {
	enabled := true
	targets, _, err := m.FindAll(ctx, TargetFilter{Enabled: &enabled}, 0, 0)
	return targets, err
}

func (m *MockTargetRepository) Update(ctx context.Context, id bson.ObjectID, target *domain.Target) error {
	if target == nil {
		return domain.ErrInvalidInput
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	idStr := id.Hex()
	existing, exists := m.targets[idStr]
	if !exists {
		return domain.ErrNotFound
	}

	for otherID, other := range m.targets {
		if otherID != idStr && other.URL == target.URL {
			return domain.ErrDuplicateURL
		}
	}

	normalizeTargetDefaults(target)
	target.ID = id
	target.CreatedAt = existing.CreatedAt
	target.UpdatedAt = time.Now().UTC()

	m.targets[idStr] = cloneTarget(target)
	return nil
}

func (m *MockTargetRepository) Delete(ctx context.Context, id bson.ObjectID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	idStr := id.Hex()
	if _, exists := m.targets[idStr]; !exists {
		return domain.ErrNotFound
	}

	delete(m.targets, idStr)
	for i, orderedID := range m.order {
		if orderedID == idStr {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}

	return nil
}

type MockCheckLogRepository struct {
	mu   sync.RWMutex
	logs []domain.CheckLog
}

func NewMockCheckLogRepository() *MockCheckLogRepository {
	return &MockCheckLogRepository{
		logs: make([]domain.CheckLog, 0),
	}
}

func (m *MockCheckLogRepository) sortLogsLocked() {
	sort.SliceStable(m.logs, func(i, j int) bool {
		if m.logs[i].CheckedAt.Equal(m.logs[j].CheckedAt) {
			return m.logs[i].ID.Hex() > m.logs[j].ID.Hex()
		}
		return m.logs[i].CheckedAt.After(m.logs[j].CheckedAt)
	})
}

func (m *MockCheckLogRepository) Create(ctx context.Context, log *domain.CheckLog) error {
	if log == nil {
		return domain.ErrInvalidInput
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if log.ID.IsZero() {
		log.ID = bson.NewObjectID()
	}
	if log.CheckedAt.IsZero() {
		log.CheckedAt = time.Now().UTC()
	} else {
		log.CheckedAt = log.CheckedAt.UTC()
	}

	m.logs = append(m.logs, *log)
	m.sortLogsLocked()
	return nil
}

func (m *MockCheckLogRepository) CreateBatch(ctx context.Context, logs []domain.CheckLog) error {
	if len(logs) == 0 {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	for i := range logs {
		if logs[i].ID.IsZero() {
			logs[i].ID = bson.NewObjectID()
		}
		if logs[i].CheckedAt.IsZero() {
			logs[i].CheckedAt = now
		} else {
			logs[i].CheckedAt = logs[i].CheckedAt.UTC()
		}
		m.logs = append(m.logs, logs[i])
	}

	m.sortLogsLocked()
	return nil
}

func (m *MockCheckLogRepository) FindByID(ctx context.Context, id bson.ObjectID) (*domain.CheckLog, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for i := range m.logs {
		if m.logs[i].ID == id {
			copied := m.logs[i]
			return &copied, nil
		}
	}

	return nil, domain.ErrNotFound
}

func matchesCheckLogFilter(l domain.CheckLog, filter CheckLogFilter) bool {
	if filter.TargetID != nil && l.TargetID != *filter.TargetID {
		return false
	}
	if filter.IsUp != nil && l.IsUp != *filter.IsUp {
		return false
	}
	if filter.StatusCode != nil && l.StatusCode != *filter.StatusCode {
		return false
	}
	if filter.From != nil && l.CheckedAt.Before(filter.From.UTC()) {
		return false
	}
	if filter.To != nil && l.CheckedAt.After(filter.To.UTC()) {
		return false
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		lowerSearch := strings.ToLower(search)
		if !strings.Contains(strings.ToLower(l.Name), lowerSearch) &&
			!strings.Contains(strings.ToLower(l.URL), lowerSearch) &&
			!strings.Contains(strings.ToLower(l.Error), lowerSearch) {
			return false
		}
	}
	return true
}

func (m *MockCheckLogRepository) FindAll(ctx context.Context, filter CheckLogFilter, limit, offset int64) ([]domain.CheckLog, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	filtered := make([]domain.CheckLog, 0, len(m.logs))
	for _, l := range m.logs {
		if matchesCheckLogFilter(l, filter) {
			filtered = append(filtered, l)
		}
	}

	total := int64(len(filtered))
	if offset < 0 {
		offset = 0
	}
	if offset >= total {
		return []domain.CheckLog{}, total, nil
	}

	end := total
	if limit > 0 && limit < total-offset {
		end = offset + limit
	}

	result := make([]domain.CheckLog, end-offset)
	copy(result, filtered[offset:end])
	return result, total, nil
}

func (m *MockCheckLogRepository) FindByTargetID(ctx context.Context, targetID bson.ObjectID, limit, offset int64) ([]domain.CheckLog, int64, error) {
	return m.FindAll(ctx, CheckLogFilter{TargetID: &targetID}, limit, offset)
}
