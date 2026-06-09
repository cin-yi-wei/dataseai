package duckdb

import "github.com/conray/dataseai/internal/db"

// BuildDSN returns the DuckDB file path from Host.
// Use ":memory:" for an in-memory database. Port and credentials are ignored;
// DuckDB is file-based with no network authentication.
func (DuckDB) BuildDSN(in db.DSNInput) string {
	if in.Network != "" {
		return in.Network
	}
	return in.Host
}
