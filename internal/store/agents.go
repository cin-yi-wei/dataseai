package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

type Agent struct {
	ID          int64      `json:"id"`
	UserID      int64      `json:"user_id"`
	Name        string     `json:"name"`
	LastSeenAt  *time.Time `json:"last_seen_at,omitempty"`
	LastIP      string     `json:"last_ip,omitempty"`
	LastOS      string     `json:"last_os,omitempty"`
	LastVersion string     `json:"last_version,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

var ErrAgentNotFound = errors.New("agent not found")

// CreateAgent registers a new agent for the given user. The plaintext token
// is returned ONCE — only its sha256 hex is persisted. Format: ag_<48 hex>.
func (s *Store) CreateAgent(userID int64, name string) (Agent, string, error) {
	if name == "" {
		return Agent{}, "", fmt.Errorf("name required")
	}
	raw := make([]byte, 24) // → 48 hex chars after encoding
	if _, err := rand.Read(raw); err != nil {
		return Agent{}, "", err
	}
	token := "ag_" + hex.EncodeToString(raw)
	hash := HashAgentToken(token)
	res, err := s.DB.Exec(
		`INSERT INTO agents (user_id, name, token_hash) VALUES (?, ?, ?)`,
		userID, name, hash,
	)
	if err != nil {
		return Agent{}, "", err
	}
	id, _ := res.LastInsertId()
	a, err := s.GetAgent(id)
	if err != nil {
		return Agent{}, "", err
	}
	return a, token, nil
}

// HashAgentToken returns the storage form of a token (hex SHA-256).
func HashAgentToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Store) GetAgent(id int64) (Agent, error) {
	return scanAgent(s.DB.QueryRow(
		`SELECT id, user_id, name, last_seen_at, last_ip, last_os, last_version, created_at
		 FROM agents WHERE id = ?`, id))
}

// GetAgentByTokenHash is what the broker calls when a connector sends Hello.
func (s *Store) GetAgentByTokenHash(hash string) (Agent, error) {
	return scanAgent(s.DB.QueryRow(
		`SELECT id, user_id, name, last_seen_at, last_ip, last_os, last_version, created_at
		 FROM agents WHERE token_hash = ?`, hash))
}

func (s *Store) ListAgents(userID int64) ([]Agent, error) {
	rows, err := s.DB.Query(
		`SELECT id, user_id, name, last_seen_at, last_ip, last_os, last_version, created_at
		 FROM agents WHERE user_id = ? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Agent
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) DeleteAgent(id, userID int64) error {
	res, err := s.DB.Exec(`DELETE FROM agents WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrAgentNotFound
	}
	return nil
}

func (s *Store) UpdateAgentLastSeen(id int64, ip, os, version string) error {
	_, err := s.DB.Exec(
		`UPDATE agents SET last_seen_at = CURRENT_TIMESTAMP,
		   last_ip = ?, last_os = ?, last_version = ?
		 WHERE id = ?`, ip, os, version, id)
	return err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanAgent(row scanner) (Agent, error) {
	var (
		a            Agent
		lastSeenStr  sql.NullString
		lastIP       sql.NullString
		lastOS       sql.NullString
		lastVer      sql.NullString
		createdAtStr string
	)
	err := row.Scan(&a.ID, &a.UserID, &a.Name, &lastSeenStr, &lastIP, &lastOS, &lastVer, &createdAtStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Agent{}, ErrAgentNotFound
		}
		return Agent{}, err
	}
	if lastSeenStr.Valid {
		if t, err := time.Parse("2006-01-02 15:04:05", lastSeenStr.String); err == nil {
			a.LastSeenAt = &t
		}
	}
	a.LastIP = lastIP.String
	a.LastOS = lastOS.String
	a.LastVersion = lastVer.String
	if t, err := time.Parse("2006-01-02 15:04:05", createdAtStr); err == nil {
		a.CreatedAt = t
	}
	return a, nil
}
