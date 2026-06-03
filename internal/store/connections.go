package store

import (
	"database/sql"
	"strings"
	"time"

	"github.com/conray/mysqlweb/internal/crypto"
)

type ConnectionInput struct {
	Name      string
	Host      string
	Port      int
	Username  string
	Password  string // empty on Update = keep existing
	DefaultDB string
	TLS       string // "disabled" | "preferred" | "required"
	Color     string
}

type Connection struct {
	ID        int64
	UserID    int64
	Name      string
	Host      string
	Port      int
	Username  string
	DefaultDB string
	TLS       string
	Color     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (s *Store) CreateConnection(c *crypto.Cipher, userID int64, in ConnectionInput) (Connection, error) {
	if in.Port == 0 {
		in.Port = 3306
	}
	if in.TLS == "" {
		in.TLS = "disabled"
	}
	enc, err := c.Encrypt([]byte(in.Password))
	if err != nil {
		return Connection{}, err
	}
	res, err := s.DB.Exec(
		`INSERT INTO connections(user_id, name, host, port, username, password_enc, default_db, tls, color)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		userID, in.Name, in.Host, in.Port, in.Username, enc, in.DefaultDB, in.TLS, in.Color,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return Connection{}, ErrDuplicate
		}
		return Connection{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetConnection(userID, id)
}

func (s *Store) GetConnection(userID, id int64) (Connection, error) {
	row := s.DB.QueryRow(
		`SELECT id, user_id, name, host, port, username, default_db, tls, color, created_at, updated_at
		 FROM connections WHERE id=? AND user_id=?`,
		id, userID,
	)
	var c Connection
	if err := row.Scan(&c.ID, &c.UserID, &c.Name, &c.Host, &c.Port, &c.Username, &c.DefaultDB, &c.TLS, &c.Color, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return c, ErrNotFound
		}
		return c, err
	}
	return c, nil
}

func (s *Store) GetConnectionPassword(c *crypto.Cipher, userID, id int64) (string, error) {
	var enc []byte
	err := s.DB.QueryRow(
		`SELECT password_enc FROM connections WHERE id=? AND user_id=?`,
		id, userID,
	).Scan(&enc)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", ErrNotFound
		}
		return "", err
	}
	pw, err := c.Decrypt(enc)
	if err != nil {
		return "", err
	}
	return string(pw), nil
}

func (s *Store) ListConnections(userID int64) ([]Connection, error) {
	rows, err := s.DB.Query(
		`SELECT id, user_id, name, host, port, username, default_db, tls, color, created_at, updated_at
		 FROM connections WHERE user_id=? ORDER BY name`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Connection
	for rows.Next() {
		var c Connection
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.Host, &c.Port, &c.Username, &c.DefaultDB, &c.TLS, &c.Color, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) UpdateConnection(c *crypto.Cipher, userID, id int64, in ConnectionInput) (Connection, error) {
	if _, err := s.GetConnection(userID, id); err != nil {
		return Connection{}, err
	}
	if in.Port == 0 {
		in.Port = 3306
	}
	if in.TLS == "" {
		in.TLS = "disabled"
	}
	if in.Password == "" {
		_, err := s.DB.Exec(
			`UPDATE connections
			 SET name=?, host=?, port=?, username=?, default_db=?, tls=?, color=?, updated_at=CURRENT_TIMESTAMP
			 WHERE id=? AND user_id=?`,
			in.Name, in.Host, in.Port, in.Username, in.DefaultDB, in.TLS, in.Color, id, userID,
		)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				return Connection{}, ErrDuplicate
			}
			return Connection{}, err
		}
	} else {
		enc, err := c.Encrypt([]byte(in.Password))
		if err != nil {
			return Connection{}, err
		}
		_, err = s.DB.Exec(
			`UPDATE connections
			 SET name=?, host=?, port=?, username=?, password_enc=?, default_db=?, tls=?, color=?, updated_at=CURRENT_TIMESTAMP
			 WHERE id=? AND user_id=?`,
			in.Name, in.Host, in.Port, in.Username, enc, in.DefaultDB, in.TLS, in.Color, id, userID,
		)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				return Connection{}, ErrDuplicate
			}
			return Connection{}, err
		}
	}
	return s.GetConnection(userID, id)
}

func (s *Store) DeleteConnection(userID, id int64) error {
	res, err := s.DB.Exec("DELETE FROM connections WHERE id=? AND user_id=?", id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
