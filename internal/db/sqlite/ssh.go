package sqlite

import (
	"errors"

	"github.com/conray/dataseai/internal/db"
)

// RegisterSSHDialer is not supported for SQLite; the engine is file-based
// and does not connect over a network socket.
func (SQLite) RegisterSSHDialer(db.SSHConfig, db.DSNInput) (string, func(), error) {
	return "", nil, errors.New("sqlite: SSH tunneling not supported for file-based databases")
}
