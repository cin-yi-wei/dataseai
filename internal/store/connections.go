package store

import (
	"database/sql"
	"strings"
	"time"

	"github.com/conray/dataseai/internal/crypto"
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
	// SSH tunnel (all empty = no tunnel)
	SSHEnabled  bool
	SSHHost     string
	SSHPort     int
	SSHUser     string
	SSHPassword string // empty on Update = keep existing
}

type Connection struct {
	ID         int64
	UserID     int64
	Name       string
	Host       string
	Port       int
	Username   string
	DefaultDB  string
	TLS        string
	Color      string
	SSHEnabled bool
	SSHHost    string
	SSHPort    int
	SSHUser    string
	CreatedAt  time.Time
	UpdatedAt  time.Time
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
	var sshEnc []byte
	if in.SSHEnabled && in.SSHPassword != "" {
		sshEnc, err = c.Encrypt([]byte(in.SSHPassword))
		if err != nil {
			return Connection{}, err
		}
	}
	sshEnabledInt := 0
	if in.SSHEnabled {
		sshEnabledInt = 1
	}
	if in.SSHPort == 0 {
		in.SSHPort = 22
	}
	res, err := s.DB.Exec(
		`INSERT INTO connections(user_id, name, host, port, username, password_enc, default_db, tls, color,
		                         ssh_enabled, ssh_host, ssh_port, ssh_user, ssh_password_enc)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		userID, in.Name, in.Host, in.Port, in.Username, enc, in.DefaultDB, in.TLS, in.Color,
		sshEnabledInt, in.SSHHost, in.SSHPort, in.SSHUser, sshEnc,
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
		`SELECT id, user_id, name, host, port, username, default_db, tls, color,
		        COALESCE(ssh_enabled, 0), COALESCE(ssh_host, ''), COALESCE(ssh_port, 22), COALESCE(ssh_user, ''),
		        created_at, updated_at
		 FROM connections WHERE id=? AND user_id=?`,
		id, userID,
	)
	var c Connection
	var sshEnabledInt int
	if err := row.Scan(&c.ID, &c.UserID, &c.Name, &c.Host, &c.Port, &c.Username, &c.DefaultDB, &c.TLS, &c.Color,
		&sshEnabledInt, &c.SSHHost, &c.SSHPort, &c.SSHUser,
		&c.CreatedAt, &c.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return c, ErrNotFound
		}
		return c, err
	}
	c.SSHEnabled = sshEnabledInt == 1
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
		`SELECT id, user_id, name, host, port, username, default_db, tls, color,
		        COALESCE(ssh_enabled, 0), COALESCE(ssh_host, ''), COALESCE(ssh_port, 22), COALESCE(ssh_user, ''),
		        created_at, updated_at
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
		var sshEnabledInt int
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.Host, &c.Port, &c.Username, &c.DefaultDB, &c.TLS, &c.Color,
			&sshEnabledInt, &c.SSHHost, &c.SSHPort, &c.SSHUser,
			&c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.SSHEnabled = sshEnabledInt == 1
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetSSHPassword reads and decrypts the SSH password for a connection.
func (s *Store) GetSSHPassword(c *crypto.Cipher, userID, id int64) (string, error) {
	var enc []byte
	err := s.DB.QueryRow(
		`SELECT ssh_password_enc FROM connections WHERE id=? AND user_id=?`,
		id, userID,
	).Scan(&enc)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", ErrNotFound
		}
		return "", err
	}
	if len(enc) == 0 {
		return "", nil
	}
	pw, err := c.Decrypt(enc)
	if err != nil {
		return "", err
	}
	return string(pw), nil
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
	if in.SSHPort == 0 {
		in.SSHPort = 22
	}
	sshEnabledInt := 0
	if in.SSHEnabled {
		sshEnabledInt = 1
	}
	if in.Password == "" {
		// Don't touch mysql password. Update SSH password only if provided.
		if in.SSHPassword == "" {
			_, err := s.DB.Exec(
				`UPDATE connections
				 SET name=?, host=?, port=?, username=?, default_db=?, tls=?, color=?,
				     ssh_enabled=?, ssh_host=?, ssh_port=?, ssh_user=?,
				     updated_at=CURRENT_TIMESTAMP
				 WHERE id=? AND user_id=?`,
				in.Name, in.Host, in.Port, in.Username, in.DefaultDB, in.TLS, in.Color,
				sshEnabledInt, in.SSHHost, in.SSHPort, in.SSHUser,
				id, userID,
			)
			if err != nil {
				if strings.Contains(err.Error(), "UNIQUE constraint failed") {
					return Connection{}, ErrDuplicate
				}
				return Connection{}, err
			}
		} else {
			sshEnc, err := c.Encrypt([]byte(in.SSHPassword))
			if err != nil {
				return Connection{}, err
			}
			_, err = s.DB.Exec(
				`UPDATE connections
				 SET name=?, host=?, port=?, username=?, default_db=?, tls=?, color=?,
				     ssh_enabled=?, ssh_host=?, ssh_port=?, ssh_user=?, ssh_password_enc=?,
				     updated_at=CURRENT_TIMESTAMP
				 WHERE id=? AND user_id=?`,
				in.Name, in.Host, in.Port, in.Username, in.DefaultDB, in.TLS, in.Color,
				sshEnabledInt, in.SSHHost, in.SSHPort, in.SSHUser, sshEnc,
				id, userID,
			)
			if err != nil {
				if strings.Contains(err.Error(), "UNIQUE constraint failed") {
					return Connection{}, ErrDuplicate
				}
				return Connection{}, err
			}
		}
	} else {
		enc, err := c.Encrypt([]byte(in.Password))
		if err != nil {
			return Connection{}, err
		}
		var sshEnc []byte
		if in.SSHPassword != "" {
			sshEnc, err = c.Encrypt([]byte(in.SSHPassword))
			if err != nil {
				return Connection{}, err
			}
		}
		// When SSH password is left blank we don't touch the stored one.
		if sshEnc != nil {
			_, err = s.DB.Exec(
				`UPDATE connections
				 SET name=?, host=?, port=?, username=?, password_enc=?, default_db=?, tls=?, color=?,
				     ssh_enabled=?, ssh_host=?, ssh_port=?, ssh_user=?, ssh_password_enc=?,
				     updated_at=CURRENT_TIMESTAMP
				 WHERE id=? AND user_id=?`,
				in.Name, in.Host, in.Port, in.Username, enc, in.DefaultDB, in.TLS, in.Color,
				sshEnabledInt, in.SSHHost, in.SSHPort, in.SSHUser, sshEnc,
				id, userID,
			)
		} else {
			_, err = s.DB.Exec(
				`UPDATE connections
				 SET name=?, host=?, port=?, username=?, password_enc=?, default_db=?, tls=?, color=?,
				     ssh_enabled=?, ssh_host=?, ssh_port=?, ssh_user=?,
				     updated_at=CURRENT_TIMESTAMP
				 WHERE id=? AND user_id=?`,
				in.Name, in.Host, in.Port, in.Username, enc, in.DefaultDB, in.TLS, in.Color,
				sshEnabledInt, in.SSHHost, in.SSHPort, in.SSHUser,
				id, userID,
			)
		}
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
