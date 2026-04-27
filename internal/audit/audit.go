// Package audit, operasyonel olayların append-only kaydını tutar.
package audit

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Action string

const (
	ActionLoginAttempt        Action = "auth.login_attempt"
	ActionDeviceCreated       Action = "device.created"
	ActionDeviceUpdated       Action = "device.updated"
	ActionDeviceDeleted       Action = "device.deleted"
	ActionSiteCreated         Action = "site.created"
	ActionTowerCreated        Action = "tower.created"
	ActionLinkCreated         Action = "link.created"
	ActionCustomerCreated     Action = "customer.created"
	ActionCredProfileCreated  Action = "credential_profile.created"
	ActionCredProfileUpdated  Action = "credential_profile.updated"
	ActionCredProfileDeleted  Action = "credential_profile.deleted"
	ActionScheduledCheckRan   Action = "scheduled_check.ran"
	ActionRecommendationMade  Action = "recommendation.made"
	ActionRecommendationApply Action = "recommendation.apply" // Faz 9
	ActionConfigBackup        Action = "config.backup"        // Faz 9
	ActionRollback            Action = "config.rollback"      // Faz 9
)

type Outcome string

const (
	OutcomeSuccess Outcome = "success"
	OutcomeFailure Outcome = "failure"
	OutcomeBlocked Outcome = "blocked"
)

type Entry struct {
	At       time.Time
	Actor    string
	Action   Action
	Subject  string
	Outcome  Outcome
	Reason   string
	Metadata map[string]any
}

type Sink interface {
	Write(ctx context.Context, e Entry) error
}

// MemorySink, sadece test/skelet kullanımı için.
type MemorySink struct {
	mu      sync.Mutex
	entries []Entry
}

func NewMemorySink() *MemorySink { return &MemorySink{} }

func (m *MemorySink) Write(ctx context.Context, e Entry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	m.entries = append(m.entries, e)
	return nil
}

func (m *MemorySink) Entries() []Entry {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Entry, len(m.entries))
	copy(out, m.entries)
	return out
}

// PostgresSink, audit_logs tablosuna yazar. Ham secret'lar
// metadata'ya konmamalıdır; çağıran taraf bunu garanti etmelidir.
type PostgresSink struct {
	P *pgxpool.Pool
}

func NewPostgresSink(p *pgxpool.Pool) *PostgresSink { return &PostgresSink{P: p} }

func (s *PostgresSink) Write(ctx context.Context, e Entry) error {
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	if e.Outcome == "" {
		e.Outcome = OutcomeSuccess
	}
	if e.Metadata == nil {
		e.Metadata = map[string]any{}
	}
	md, _ := json.Marshal(e.Metadata)
	_, err := s.P.Exec(ctx, `
INSERT INTO audit_logs(at, actor, action, subject, outcome, reason, metadata)
VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		e.At, e.Actor, string(e.Action), e.Subject, string(e.Outcome), e.Reason, md,
	)
	return err
}
