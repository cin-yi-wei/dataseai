package store

import (
	"database/sql"
	"errors"
	"time"
)

// AIPolicy holds the four write-permission flags for a single table.
type AIPolicy struct {
	Insert bool `json:"insert"`
	Update bool `json:"update"`
	Delete bool `json:"delete"`
	DDL    bool `json:"ddl"`
}

// Policy scope discriminator. The same (user, conn, db, table) can hold one
// AI-scoped policy and one DML-scoped policy, with independent flags.
type PolicyScope string

const (
	ScopeAI  PolicyScope = "ai"
	ScopeDML PolicyScope = "dml"
)

// NormalizeScope coerces an external string (query param, JSON field) to a
// known scope, falling back to ScopeAI for backward compatibility.
func NormalizeScope(s string) PolicyScope {
	if s == string(ScopeDML) {
		return ScopeDML
	}
	return ScopeAI
}

// AITablePolicy pairs a table name with its policy, used in list responses.
type AITablePolicy struct {
	Table  string   `json:"table"`
	Policy AIPolicy `json:"policy"`
}

func boolI(b bool) int {
	if b {
		return 1
	}
	return 0
}

// GetWritePolicy returns (policy, found, err) for a specific scope. When not
// found, policy is zero. The 'ai' and 'dml' scopes are independent rows.
func (s *Store) GetWritePolicy(userID, connID int64, db, table string, scope PolicyScope) (AIPolicy, bool, error) {
	var p AIPolicy
	var ai, au, ad, addl int
	row := s.DB.QueryRow(`
		SELECT allow_insert, allow_update, allow_delete, allow_ddl
		  FROM ai_write_policy
		 WHERE user_id=? AND connection_id=? AND database_name=? AND table_name=? AND scope=?`,
		userID, connID, db, table, string(scope))
	if err := row.Scan(&ai, &au, &ad, &addl); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return p, false, nil
		}
		return p, false, err
	}
	p.Insert = ai == 1
	p.Update = au == 1
	p.Delete = ad == 1
	p.DDL = addl == 1
	return p, true, nil
}

// UpsertWritePolicy inserts or fully replaces the policy for one (table, scope).
func (s *Store) UpsertWritePolicy(userID, connID int64, db, table string, scope PolicyScope, p AIPolicy) error {
	_, err := s.DB.Exec(`
		INSERT INTO ai_write_policy (user_id, connection_id, database_name, table_name, scope,
		    allow_insert, allow_update, allow_delete, allow_ddl, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(user_id, connection_id, database_name, table_name, scope) DO UPDATE SET
		    allow_insert = excluded.allow_insert,
		    allow_update = excluded.allow_update,
		    allow_delete = excluded.allow_delete,
		    allow_ddl    = excluded.allow_ddl,
		    updated_at   = excluded.updated_at`,
		userID, connID, db, table, string(scope),
		boolI(p.Insert), boolI(p.Update), boolI(p.Delete), boolI(p.DDL),
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

// BatchUpsertWritePolicy applies the same policy to multiple tables (one scope) in a single tx.
func (s *Store) BatchUpsertWritePolicy(userID, connID int64, db string, tables []string, scope PolicyScope, p AIPolicy) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.Prepare(`
		INSERT INTO ai_write_policy (user_id, connection_id, database_name, table_name, scope,
		    allow_insert, allow_update, allow_delete, allow_ddl, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(user_id, connection_id, database_name, table_name, scope) DO UPDATE SET
		    allow_insert = excluded.allow_insert,
		    allow_update = excluded.allow_update,
		    allow_delete = excluded.allow_delete,
		    allow_ddl    = excluded.allow_ddl,
		    updated_at   = excluded.updated_at`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, t := range tables {
		if _, err := stmt.Exec(userID, connID, db, t, string(scope),
			boolI(p.Insert), boolI(p.Update), boolI(p.Delete), boolI(p.DDL), now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListWritePolicy returns all per-table policies for a user+connection+database+scope.
func (s *Store) ListWritePolicy(userID, connID int64, db string, scope PolicyScope) ([]AITablePolicy, error) {
	rows, err := s.DB.Query(`
		SELECT table_name, allow_insert, allow_update, allow_delete, allow_ddl
		  FROM ai_write_policy
		 WHERE user_id=? AND connection_id=? AND database_name=? AND scope=?
		 ORDER BY table_name`,
		userID, connID, db, string(scope))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AITablePolicy
	for rows.Next() {
		var tp AITablePolicy
		var ai, au, ad, addl int
		if err := rows.Scan(&tp.Table, &ai, &au, &ad, &addl); err != nil {
			return nil, err
		}
		tp.Policy = AIPolicy{Insert: ai == 1, Update: au == 1, Delete: ad == 1, DDL: addl == 1}
		out = append(out, tp)
	}
	return out, rows.Err()
}

// DeleteWritePolicy removes the policy row for one (table, scope). No error if missing.
func (s *Store) DeleteWritePolicy(userID, connID int64, db, table string, scope PolicyScope) error {
	_, err := s.DB.Exec(`
		DELETE FROM ai_write_policy
		 WHERE user_id=? AND connection_id=? AND database_name=? AND table_name=? AND scope=?`,
		userID, connID, db, table, string(scope))
	return err
}

// Backward-compat wrappers — existing callers pass no scope and get the AI one.
func (s *Store) GetAIPolicy(userID, connID int64, db, table string) (AIPolicy, bool, error) {
	return s.GetWritePolicy(userID, connID, db, table, ScopeAI)
}
func (s *Store) UpsertAIPolicy(userID, connID int64, db, table string, p AIPolicy) error {
	return s.UpsertWritePolicy(userID, connID, db, table, ScopeAI, p)
}
func (s *Store) BatchUpsertAIPolicy(userID, connID int64, db string, tables []string, p AIPolicy) error {
	return s.BatchUpsertWritePolicy(userID, connID, db, tables, ScopeAI, p)
}
func (s *Store) ListAIPolicy(userID, connID int64, db string) ([]AITablePolicy, error) {
	return s.ListWritePolicy(userID, connID, db, ScopeAI)
}
func (s *Store) DeleteAIPolicy(userID, connID int64, db, table string) error {
	return s.DeleteWritePolicy(userID, connID, db, table, ScopeAI)
}
