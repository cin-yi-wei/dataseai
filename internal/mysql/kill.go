package mysql

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"sync"
	"time"
)

type ActiveQuery struct {
	QueryID      string    `json:"query_id"`
	ConnectionID int64     `json:"-"`
	SQLExcerpt   string    `json:"sql_excerpt"`
	UserID       int64     `json:"-"`
	ConnID       int64     `json:"conn_id"`
	StartedAt    time.Time `json:"started_at"`
}

type Registry struct {
	mu sync.Mutex
	m  map[string]ActiveQuery
}

var ErrNoMatch = errors.New("no matching active query")

func NewRegistry() *Registry {
	return &Registry{m: map[string]ActiveQuery{}}
}

func (r *Registry) Register(queryID string, connectionID int64, sqlText string, userID, connID int64) {
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

func (r *Registry) Unregister(queryID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.m, queryID)
}

func (r *Registry) List(userID int64) []ActiveQuery {
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

func (r *Registry) Find(userID int64, queryID string) (ActiveQuery, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	q, ok := r.m[queryID]
	if !ok || q.UserID != userID {
		return ActiveQuery{}, false
	}
	return q, true
}

func (r *Registry) KillByQueryID(ctx context.Context, db *sql.DB, userID int64, queryID string) error {
	entry, ok := r.Find(userID, queryID)
	if !ok {
		return ErrNoMatch
	}
	_, err := db.ExecContext(ctx, "KILL QUERY "+strconv.FormatInt(entry.ConnectionID, 10))
	return err
}

func excerpt(sqlText string) string {
	if len(sqlText) <= 200 {
		return sqlText
	}
	return sqlText[:200] + "..."
}
