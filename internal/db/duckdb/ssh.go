package duckdb

import (
	"errors"

	"github.com/conray/dataseai/internal/db"
)

func (DuckDB) RegisterSSHDialer(db.SSHConfig, db.DSNInput) (string, func(), error) {
	return "", nil, errors.New("duckdb: SSH tunneling not supported for file-based databases")
}
