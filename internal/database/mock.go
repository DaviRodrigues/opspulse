package database

import (
	"context"
	"sync"
	"time"

	"github.com/DaviRodrigues/opspulse/internal/domain"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type MockTargetRepository struct {
	mu      sync.RWMutex
	targets map[string]*domain.Target
	order   []string
}

func NewMockTargetRepository() *MockTargetRepository {
	return &MockTargetRepository{
		targets: make(map[string]*domain.Target),
	}
}

func (m *MockTargetRepository) Create(ctx context.Context, target *domain.Target) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if target.ID.IsZero() {
		target.ID = bson.NewObjectID()
	}
	now := time.Now().UTC()
	if target.CreatedAt.IsZero() {
		target.CreatedAt = now
	}
	target.UpdatedAt = now

	idStr := target.ID.Hex()
	m.targets[idStr] = target

	return nil
}

func (m *MockTargetRepository) FindByID(ctx context.Context, id bson.ObjectID) (*domain.Target, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	target, exists := m.targets[id.Hex()]
	if !exists {
		return nil, domain.ErrNotFound
	}

	copied := target
	return copied, nil
}

func (m *MockTargetRepository) FindAll(ctx context.Context, limit, offset int64) ([]*domain.Target, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	total := int64(len(m.order))
	if offset >= total {
		return []*domain.Target{}, total, nil
	}

	end := total
	if limit > 0 && offset+limit < total {
		end = offset + limit
	}

	result := make([]*domain.Target, 0, end-offset)
	for i := offset; i < end; i++ {
		idStr := m.order[i]
		result = append(result, m.targets[idStr])
	}

	return result, total, nil
}

func (m *MockTargetRepository) Update(ctx context.Context, id bson.ObjectID, target *domain.Target) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	idStr := id.Hex()
	existing, exists := m.targets[idStr]
	if !exists {
		return domain.ErrNotFound
	}

	target.ID = id
	target.CreatedAt = existing.CreatedAt
	target.UpdatedAt = time.Now().UTC()

	m.targets[idStr] = target
	return nil
}

type MockCheckLogRepository struct {
	mu   sync.RWMutex
	logs []domain.CheckLogs
}

func NewMockCheckLogRepository() *MockCheckLogRepository {
	return &MockCheckLogRepository{
		logs: make([]domain.CheckLogs, 0),
	}
}

func (m *MockCheckLogRepository) Create(ctx context.Context, log *domain.CheckLogs) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if log.ID.IsZero() {
		log.ID = bson.NewObjectID()
	}
	if log.CheckedAt.IsZero() {
		log.CheckedAt = time.Now().UTC()
	}

	// Inserir no início para manter ordenação decrescente por checked_at
	m.logs = append([]domain.CheckLogs{*log}, m.logs...)
	return nil
}

func (m *MockCheckLogRepository) CreateBatch(ctx context.Context, logs []domain.CheckLogs) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	toPrepend := make([]domain.CheckLogs, len(logs))
	for i := range logs {
		if logs[i].ID.IsZero() {
			logs[i].ID = bson.NewObjectID()
		}
		if logs[i].CheckedAt.IsZero() {
			logs[i].CheckedAt = now
		}
		toPrepend[i] = logs[i]
	}

	m.logs = append(toPrepend, m.logs...)
	return nil
}

func (m *MockCheckLogRepository) FindAll(ctx context.Context, limit, offset int64) ([]domain.CheckLogs, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	total := int64(len(m.logs))
	if offset >= total {
		return []domain.CheckLogs{}, total, nil
	}

	end := total
	if limit > 0 && offset+limit < total {
		end = offset + limit
	}

	result := make([]domain.CheckLogs, end-offset)
	copy(result, m.logs[offset:end])
	return result, total, nil
}

func (m *MockCheckLogRepository) FindByTargetID(ctx context.Context, targetID bson.ObjectID, limit, offset int64) ([]domain.CheckLogs, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var filtered []domain.CheckLogs
	for _, l := range m.logs {
		if l.TargetID == targetID {
			filtered = append(filtered, l)
		}
	}

	total := int64(len(filtered))
	if offset >= total {
		return []domain.CheckLogs{}, total, nil
	}

	end := total
	if limit > 0 && offset+limit < total {
		end = offset + limit
	}

	result := make([]domain.CheckLogs, end-offset)
	copy(result, filtered[offset:end])
	return result, total, nil
}
