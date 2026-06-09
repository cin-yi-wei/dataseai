package db

import (
	"errors"
	"sync"
	"time"
)

// ActiveQuery captures the bookkeeping needed to kill a running query
// without re-querying the DB. Engine-agnostic: each dialect interprets
// ConnectionID per its own KILL semantics.
type ActiveQuery struct {
	QueryID      string    `json:"query_id"`
	ConnectionID int64     `json:"-"`
	SQLExcerpt   string    `json:"sql_excerpt"`
	UserID       int64     `json:"-"`
	ConnID       int64     `json:"conn_id"`
	StartedAt    time.Time `json:"started_at"`
}

// KillRegistry is a process-wide map of in-flight queries. The dialect's
// KillQuery method consumes the ConnectionID to issue the engine-specific
// cancellation statement.
type KillRegistry struct {
	mu sync.Mutex
	m  map[string]ActiveQuery
}

// ErrKillNoMatch is returned when a kill targets a non-existent or
// foreign-owned query.
var ErrKillNoMatch = errors.New("no matching active query")

func NewKillRegistry() *KillRegistry {
	return &KillRegistry{m: map[string]ActiveQuery{}}
}

func (r *KillRegistry) Register(queryID string, connectionID int64, sqlText string, userID, connID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[queryID] = ActiveQuery{
		QueryID:      queryID,
		ConnectionID: connectionID,
		SQLExcerpt:   excerpt(sqlText),
		UserID:       userID,
		ConnID:       connID,
		StartedAt:    time.Now(),
	}
}

func (r *KillRegistry) Unregister(queryID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.m, queryID)
}

func (r *KillRegistry) Lookup(queryID string) (ActiveQuery, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	q, ok := r.m[queryID]
	return q, ok
}

// List returns every active query owned by userID. The API
// /api/queries/active endpoint surfaces this so users see only their own
// in-flight queries.
func (r *KillRegistry) List(userID int64) []ActiveQuery {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ActiveQuery, 0)
	for _, q := range r.m {
		if q.UserID == userID {
			out = append(out, q)
		}
	}
	return out
}

// Authorize verifies the query belongs to the requesting user and returns
// it for the caller to feed to dialect.KillQuery.
func (r *KillRegistry) Authorize(queryID string, userID int64) (ActiveQuery, error) {
	q, ok := r.Lookup(queryID)
	if !ok || q.UserID != userID {
		return ActiveQuery{}, ErrKillNoMatch
	}
	return q, nil
}

func excerpt(s string) string {
	const max = 200
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
