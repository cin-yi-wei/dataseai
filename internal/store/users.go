package store

import (
	"database/sql"
	"errors"
	"strings"

	"github.com/conray/dataseai/internal/crypto"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrDuplicate          = errors.New("duplicate")
	ErrNotFound           = errors.New("not found")
	ErrInvalidCredentials = errors.New("invalid credentials")
)

// dummyBcryptHash is used to defeat user-enumeration timing attacks on
// VerifyPassword. CompareHashAndPassword against this hash takes the same
// ~80ms as a real bcrypt check; the result is discarded.
//
// This is bcrypt.GenerateFromPassword([]byte("never-matches-anything"), 10).
// Generated once offline; baked into source so startup stays fast.
const dummyBcryptHash = "$2a$10$gNiXr6e1P/9/fPGvCRjxbuTd9eVYCcxBiE8ZfwYxQEZEKWAph0hzm"

type Store struct {
	DB *sql.DB
}

type User struct {
	ID       int64
	Username string
	IsAdmin  bool
}

type UserInfo struct {
	ID           int64   `json:"id"`
	Username     string  `json:"username"`
	IsAdmin      bool    `json:"is_admin"`
	CreatedAt    string  `json:"created_at"`
	ConnCount    int     `json:"conn_count"`
	SessionCount int     `json:"session_count"`
	LastSeenAt   *string `json:"last_seen_at"`
}

func (s *Store) CreateUser(username, password string) (User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}
	// First user becomes admin.
	var count int
	_ = s.DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	isAdmin := count == 0
	adminVal := 0
	if isAdmin {
		adminVal = 1
	}
	res, err := s.DB.Exec("INSERT INTO users(username, password_hash, is_admin) VALUES(?, ?, ?)", username, string(hash), adminVal)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return User{}, ErrDuplicate
		}
		return User{}, err
	}
	id, _ := res.LastInsertId()
	return User{ID: id, Username: username, IsAdmin: isAdmin}, nil
}

func (s *Store) GetUserByID(id int64) (User, error) {
	row := s.DB.QueryRow("SELECT id, username, COALESCE(is_admin, 0) FROM users WHERE id=?", id)
	var u User
	var admin int
	if err := row.Scan(&u.ID, &u.Username, &admin); err != nil {
		if err == sql.ErrNoRows {
			return u, ErrNotFound
		}
		return u, err
	}
	u.IsAdmin = admin == 1
	return u, nil
}

func (s *Store) VerifyPassword(username, password string) (User, error) {
	row := s.DB.QueryRow("SELECT id, username, COALESCE(is_admin, 0), password_hash FROM users WHERE username=?", username)
	var u User
	var admin int
	var hash string
	if err := row.Scan(&u.ID, &u.Username, &admin, &hash); err != nil {
		if err == sql.ErrNoRows {
			// Run bcrypt against a dummy hash so unknown-user timing
			// matches the wrong-password path. Defeats user enumeration.
			_ = bcrypt.CompareHashAndPassword([]byte(dummyBcryptHash), []byte(password))
			return u, ErrInvalidCredentials
		}
		return u, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return User{}, ErrInvalidCredentials
	}
	u.IsAdmin = admin == 1
	return u, nil
}

func (s *Store) UpdatePassword(userID int64, newPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec("UPDATE users SET password_hash=? WHERE id=?", string(hash), userID)
	return err
}

// Admin operations

func (s *Store) ListUsers() ([]UserInfo, error) {
	rows, err := s.DB.Query(`
		SELECT u.id, u.username, COALESCE(u.is_admin, 0), u.created_at,
		       (SELECT COUNT(*) FROM connections c WHERE c.user_id = u.id) as conn_count,
		       (SELECT COUNT(*) FROM sessions sess WHERE sess.user_id = u.id) as session_count,
		       (SELECT MAX(sess.last_used_at) FROM sessions sess WHERE sess.user_id = u.id) as last_seen_at
		FROM users u
		ORDER BY u.id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UserInfo
	for rows.Next() {
		var u UserInfo
		var admin int
		var lastSeen sql.NullString
		if err := rows.Scan(&u.ID, &u.Username, &admin, &u.CreatedAt, &u.ConnCount, &u.SessionCount, &lastSeen); err != nil {
			return nil, err
		}
		u.IsAdmin = admin == 1
		if lastSeen.Valid {
			u.LastSeenAt = &lastSeen.String
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) DeleteUser(id int64) error {
	_, err := s.DB.Exec("DELETE FROM sessions WHERE user_id=?", id)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec("DELETE FROM connections WHERE user_id=?", id)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec("DELETE FROM query_history WHERE user_id=?", id)
	if err != nil {
		return err
	}
	res, err := s.DB.Exec("DELETE FROM users WHERE id=?", id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetUserAdmin(id int64, isAdmin bool) error {
	v := 0
	if isAdmin {
		v = 1
	}
	res, err := s.DB.Exec("UPDATE users SET is_admin=? WHERE id=?", v, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

type AdminStats struct {
	TotalUsers       int `json:"total_users"`
	TotalAdmins      int `json:"total_admins"`
	TotalConnections int `json:"total_connections"`
	TotalSessions    int `json:"total_sessions"`
	TotalQueries     int `json:"total_queries"`
}

func (s *Store) GetAdminStats() (AdminStats, error) {
	var st AdminStats
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&st.TotalUsers); err != nil {
		return st, err
	}
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM users WHERE is_admin=1").Scan(&st.TotalAdmins); err != nil {
		return st, err
	}
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM connections").Scan(&st.TotalConnections); err != nil {
		return st, err
	}
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&st.TotalSessions); err != nil {
		return st, err
	}
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM query_history").Scan(&st.TotalQueries); err != nil {
		return st, err
	}
	return st, nil
}

type ConnectionInfo struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"user_id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	DBUser    string `json:"db_username"`
	DefaultDB string `json:"default_db"`
	TLS       string `json:"tls"`
	CreatedAt string `json:"created_at"`
}

func (s *Store) ListAllConnections() ([]ConnectionInfo, error) {
	rows, err := s.DB.Query(`
		SELECT c.id, c.user_id, u.username, c.name, c.host, c.port, c.username, COALESCE(c.default_db, ''), c.tls, c.created_at
		FROM connections c
		LEFT JOIN users u ON u.id = c.user_id
		ORDER BY c.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ConnectionInfo
	for rows.Next() {
		var c ConnectionInfo
		if err := rows.Scan(&c.ID, &c.UserID, &c.Username, &c.Name, &c.Host, &c.Port, &c.DBUser, &c.DefaultDB, &c.TLS, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UserAPIKeys is the per-user LLM API key set (decrypted plaintext).
type UserAPIKeys struct {
	Anthropic string
	OpenAI    string
	Gemini    string
}

// GetUserAPIKeys reads and decrypts the per-user API keys. Empty strings mean unset.
func (s *Store) GetUserAPIKeys(cipher *crypto.Cipher, userID int64) (UserAPIKeys, error) {
	var aEnc, oEnc, gEnc []byte
	err := s.DB.QueryRow(
		`SELECT anthropic_api_key_enc, openai_api_key_enc, gemini_api_key_enc FROM users WHERE id=?`,
		userID,
	).Scan(&aEnc, &oEnc, &gEnc)
	if err != nil {
		if err == sql.ErrNoRows {
			return UserAPIKeys{}, ErrNotFound
		}
		return UserAPIKeys{}, err
	}
	out := UserAPIKeys{}
	if len(aEnc) > 0 {
		if pt, err := cipher.Decrypt(aEnc); err == nil {
			out.Anthropic = string(pt)
		}
	}
	if len(oEnc) > 0 {
		if pt, err := cipher.Decrypt(oEnc); err == nil {
			out.OpenAI = string(pt)
		}
	}
	if len(gEnc) > 0 {
		if pt, err := cipher.Decrypt(gEnc); err == nil {
			out.Gemini = string(pt)
		}
	}
	return out, nil
}

// GetAIWritesEnabled returns the user's master AI-writes opt-in flag.
func (s *Store) GetAIWritesEnabled(userID int64) (bool, error) {
	var v int
	err := s.DB.QueryRow("SELECT COALESCE(ai_writes_enabled, 0) FROM users WHERE id=?", userID).Scan(&v)
	if err != nil {
		return false, err
	}
	return v == 1, nil
}

// SetAIWritesEnabled toggles the master AI-writes opt-in flag.
func (s *Store) SetAIWritesEnabled(userID int64, enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	_, err := s.DB.Exec("UPDATE users SET ai_writes_enabled=? WHERE id=?", v, userID)
	return err
}

// SetUserAPIKey stores an encrypted API key for the given provider.
// Empty string clears it.
func (s *Store) SetUserAPIKey(cipher *crypto.Cipher, userID int64, provider, key string) error {
	col := ""
	switch provider {
	case "anthropic":
		col = "anthropic_api_key_enc"
	case "openai":
		col = "openai_api_key_enc"
	case "gemini":
		col = "gemini_api_key_enc"
	default:
		return errors.New("unknown provider")
	}
	var encBytes []byte
	if key != "" {
		ct, err := cipher.Encrypt([]byte(key))
		if err != nil {
			return err
		}
		encBytes = ct
	}
	_, err := s.DB.Exec("UPDATE users SET "+col+"=? WHERE id=?", encBytes, userID)
	return err
}
