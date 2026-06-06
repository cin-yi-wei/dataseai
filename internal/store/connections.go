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
	SSHEnabled       bool
	SSHHost          string
	SSHPort          int
	SSHUser          string
	SSHPassword      string // empty on Update = keep existing
	SSHKey           string // PEM. empty on Update = keep existing
	SSHKeyPassphrase string // empty on Update = keep existing
	ViaAgentID       *int64
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
	SSHKeySet  bool // true if a private key is stored (we never expose the key itself)
	ViaAgentID *int64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

const connectionColumns = `id, user_id, name, host, port, username, default_db, tls, color,
        COALESCE(ssh_enabled, 0), COALESCE(ssh_host, ''), COALESCE(ssh_port, 22), COALESCE(ssh_user, ''),
        CASE WHEN ssh_key_enc IS NOT NULL AND length(ssh_key_enc) > 0 THEN 1 ELSE 0 END,
        via_agent_id,
        created_at, updated_at`

func scanConnection(row interface {
	Scan(dest ...any) error
}) (Connection, error) {
	var c Connection
	var sshEnabledInt, sshKeySetInt int
	var viaAgentID sql.NullInt64
	if err := row.Scan(&c.ID, &c.UserID, &c.Name, &c.Host, &c.Port, &c.Username, &c.DefaultDB, &c.TLS, &c.Color,
		&sshEnabledInt, &c.SSHHost, &c.SSHPort, &c.SSHUser, &sshKeySetInt,
		&viaAgentID, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return c, err
	}
	c.SSHEnabled = sshEnabledInt == 1
	c.SSHKeySet = sshKeySetInt == 1
	if viaAgentID.Valid {
		c.ViaAgentID = &viaAgentID.Int64
	}
	return c, nil
}

func (s *Store) CreateConnection(c *crypto.Cipher, userID int64, in ConnectionInput) (Connection, error) {
	if in.Port == 0 {
		in.Port = 3306
	}
	if in.TLS == "" {
		in.TLS = "disabled"
	}
	if in.SSHPort == 0 {
		in.SSHPort = 22
	}
	enc, err := c.Encrypt([]byte(in.Password))
	if err != nil {
		return Connection{}, err
	}
	var sshPwEnc, sshKeyEnc, sshKeyPassEnc []byte
	if in.SSHEnabled && in.SSHPassword != "" {
		sshPwEnc, err = c.Encrypt([]byte(in.SSHPassword))
		if err != nil {
			return Connection{}, err
		}
	}
	if in.SSHEnabled && in.SSHKey != "" {
		sshKeyEnc, err = c.Encrypt([]byte(in.SSHKey))
		if err != nil {
			return Connection{}, err
		}
	}
	if in.SSHEnabled && in.SSHKeyPassphrase != "" {
		sshKeyPassEnc, err = c.Encrypt([]byte(in.SSHKeyPassphrase))
		if err != nil {
			return Connection{}, err
		}
	}
	sshEnabledInt := 0
	if in.SSHEnabled {
		sshEnabledInt = 1
	}
	res, err := s.DB.Exec(
		`INSERT INTO connections(user_id, name, host, port, username, password_enc, default_db, tls, color,
		                         ssh_enabled, ssh_host, ssh_port, ssh_user,
		                         ssh_password_enc, ssh_key_enc, ssh_key_passphrase_enc, via_agent_id)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		userID, in.Name, in.Host, in.Port, in.Username, enc, in.DefaultDB, in.TLS, in.Color,
		sshEnabledInt, in.SSHHost, in.SSHPort, in.SSHUser,
		sshPwEnc, sshKeyEnc, sshKeyPassEnc, in.ViaAgentID,
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
		`SELECT `+connectionColumns+` FROM connections WHERE id=? AND user_id=?`,
		id, userID,
	)
	c, err := scanConnection(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return Connection{}, ErrNotFound
		}
		return Connection{}, err
	}
	return c, nil
}

func (s *Store) GetConnectionPassword(c *crypto.Cipher, userID, id int64) (string, error) {
	return s.decryptColumn(c, userID, id, "password_enc")
}

func (s *Store) ListConnections(userID int64) ([]Connection, error) {
	rows, err := s.DB.Query(
		`SELECT `+connectionColumns+` FROM connections WHERE user_id=? ORDER BY name`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Connection
	for rows.Next() {
		c, err := scanConnection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetSSHPassword reads and decrypts the SSH password for a connection.
func (s *Store) GetSSHPassword(c *crypto.Cipher, userID, id int64) (string, error) {
	return s.decryptColumn(c, userID, id, "ssh_password_enc")
}

// GetSSHKey returns the PEM-encoded private key and its passphrase (if any).
// Either or both may be empty.
func (s *Store) GetSSHKey(c *crypto.Cipher, userID, id int64) (key, passphrase string, err error) {
	key, err = s.decryptColumn(c, userID, id, "ssh_key_enc")
	if err != nil {
		return "", "", err
	}
	passphrase, err = s.decryptColumn(c, userID, id, "ssh_key_passphrase_enc")
	if err != nil {
		return "", "", err
	}
	return key, passphrase, nil
}

// decryptColumn reads an encrypted blob column from the connection row.
// Returns empty string (no error) when the row exists but the column is NULL/empty.
func (s *Store) decryptColumn(c *crypto.Cipher, userID, id int64, column string) (string, error) {
	var enc []byte
	err := s.DB.QueryRow(
		`SELECT `+column+` FROM connections WHERE id=? AND user_id=?`,
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

	// 1. Update the non-credential fields in one statement.
	if _, err := s.DB.Exec(
		`UPDATE connections
		 SET name=?, host=?, port=?, username=?, default_db=?, tls=?, color=?,
		     ssh_enabled=?, ssh_host=?, ssh_port=?, ssh_user=?,
		     via_agent_id=?,
		     updated_at=CURRENT_TIMESTAMP
		 WHERE id=? AND user_id=?`,
		in.Name, in.Host, in.Port, in.Username, in.DefaultDB, in.TLS, in.Color,
		sshEnabledInt, in.SSHHost, in.SSHPort, in.SSHUser,
		in.ViaAgentID,
		id, userID,
	); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return Connection{}, ErrDuplicate
		}
		return Connection{}, err
	}

	// 2. Conditionally update each credential — empty input means "keep".
	if err := s.updateCredential(c, userID, id, "password_enc", in.Password, false); err != nil {
		return Connection{}, err
	}
	if err := s.updateCredential(c, userID, id, "ssh_password_enc", in.SSHPassword, false); err != nil {
		return Connection{}, err
	}
	if err := s.updateCredential(c, userID, id, "ssh_key_enc", in.SSHKey, false); err != nil {
		return Connection{}, err
	}
	if err := s.updateCredential(c, userID, id, "ssh_key_passphrase_enc", in.SSHKeyPassphrase, false); err != nil {
		return Connection{}, err
	}

	return s.GetConnection(userID, id)
}

// updateCredential writes a single encrypted column when value is non-empty.
// When value is empty and clear=false (the usual case), the column is left
// untouched. clear=true wipes the column to NULL.
func (s *Store) updateCredential(c *crypto.Cipher, userID, id int64, column, value string, clear bool) error {
	if value == "" && !clear {
		return nil
	}
	var enc []byte
	if value != "" {
		var err error
		enc, err = c.Encrypt([]byte(value))
		if err != nil {
			return err
		}
	}
	_, err := s.DB.Exec(
		`UPDATE connections SET `+column+`=?, updated_at=CURRENT_TIMESTAMP WHERE id=? AND user_id=?`,
		enc, id, userID,
	)
	return err
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
