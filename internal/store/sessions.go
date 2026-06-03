package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"
)

var ErrSessionExpired = errors.New("session expired")

type Session struct {
	Token      string
	UserID     int64
	CreatedAt  time.Time
	LastUsedAt time.Time
	UserAgent  string
	ExpiresAt  time.Time
}

func newToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func (s *Store) CreateSession(userID int64, ua string, ttl time.Duration) (Session, error) {
	tok, err := newToken()
	if err != nil {
		return Session{}, err
	}
	exp := time.Now().Add(ttl)
	if _, err := s.DB.Exec(
		"INSERT INTO sessions(token, user_id, user_agent, expires_at) VALUES(?,?,?,?)",
		tok, userID, ua, exp,
	); err != nil {
		return Session{}, err
	}
	return Session{Token: tok, UserID: userID, UserAgent: ua, ExpiresAt: exp}, nil
}

func (s *Store) GetSession(token string) (Session, error) {
	row := s.DB.QueryRow(
		"SELECT token, user_id, created_at, last_used_at, user_agent, expires_at FROM sessions WHERE token=?",
		token,
	)
	var sess Session
	if err := row.Scan(&sess.Token, &sess.UserID, &sess.CreatedAt, &sess.LastUsedAt, &sess.UserAgent, &sess.ExpiresAt); err != nil {
		if err == sql.ErrNoRows {
			return sess, ErrNotFound
		}
		return sess, err
	}
	if time.Now().After(sess.ExpiresAt) {
		return sess, ErrSessionExpired
	}
	return sess, nil
}

func (s *Store) RefreshSession(token string, ttl time.Duration) error {
	now := time.Now()
	_, err := s.DB.Exec(
		"UPDATE sessions SET last_used_at=?, expires_at=? WHERE token=?",
		now, now.Add(ttl), token,
	)
	return err
}

func (s *Store) DeleteSession(token string) error {
	_, err := s.DB.Exec("DELETE FROM sessions WHERE token=?", token)
	return err
}

func (s *Store) DeleteUserSessionsExcept(userID int64, keepToken string) error {
	_, err := s.DB.Exec("DELETE FROM sessions WHERE user_id=? AND token<>?", userID, keepToken)
	return err
}

func (s *Store) ListSessionsByUser(userID int64) ([]Session, error) {
	rows, err := s.DB.Query(
		"SELECT token, user_id, created_at, last_used_at, user_agent, expires_at FROM sessions WHERE user_id=? ORDER BY last_used_at DESC",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var sess Session
		if err := rows.Scan(&sess.Token, &sess.UserID, &sess.CreatedAt, &sess.LastUsedAt, &sess.UserAgent, &sess.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}
