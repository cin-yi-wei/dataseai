package sqlite

import "github.com/conray/dataseai/internal/db"

// BuildDSN returns the SQLite file path. The Host field holds the path to the
// .sqlite/.db file (e.g. "/var/data/app.db" or ":memory:"). Port and network
// credentials are ignored — SQLite is file-based.
//
// When in.Network is set (used for SSH passthrough or test overrides), it is
// returned verbatim.
func (SQLite) BuildDSN(in db.DSNInput) string {
	if in.Network != "" {
		return in.Network
	}
	return in.Host
}
