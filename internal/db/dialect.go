package db

import (
	"context"
	"database/sql"
)

// Dialect collects every engine-specific behavior the platform depends on.
// Adding a new database means writing a new Dialect implementation and
// registering it in this package's init chain — no consumer touches the
// engine name directly.
//
// Methods grouped by concern:
//   - Identity:      Engine, DriverName
//   - Connection:    BuildDSN, RegisterSSHDialer
//   - Quoting:       QuoteIdent, Placeholder
//   - Parsing:       ClassifySQL
//   - Discovery:     ListDatabases, ListTables, ListSchemaColumns,
//                    DescribeTable, ListIndexes, ListForeignKeys
//   - Browsing:      FetchTableRows
//   - DML:           PrimaryKey, UpdateCell, InsertRow, DeleteRow
//   - Admin:         KillQuery, ConnectionIDQuery
type Dialect interface {
	// Engine returns the canonical engine name (e.g. "mysql").
	Engine() Engine
	// DriverName is the string passed to sql.Open (e.g. "mysql", "pgx", "sqlserver").
	DriverName() string
	// BuildDSN turns generic input into a driver-specific DSN string.
	BuildDSN(DSNInput) string
	// RegisterSSHDialer wires an SSH-tunneled dialer into the underlying driver
	// and returns a name suitable for passing back into DSNInput.Network so
	// the next BuildDSN routes through the tunnel. Returns a closer the pool
	// invokes when the entry is evicted.
	RegisterSSHDialer(SSHConfig) (name string, closer func(), err error)

	// QuoteIdent wraps a user-controlled identifier in the dialect's
	// quoting style. Required to be SQL-injection-safe.
	QuoteIdent(string) string
	// Placeholder returns the placeholder string for parameter index n
	// (1-based for engines that care, ignored otherwise).
	Placeholder(n int) string

	// ClassifySQL parses a statement well enough to enforce policy:
	// op kind, target table, multi-statement flag.
	ClassifySQL(string) (Classified, error)

	ListDatabases(ctx context.Context, db *sql.DB, includeSystem bool) ([]string, error)
	ListTables(ctx context.Context, db *sql.DB, schema string) ([]TableInfo, error)
	ListSchemaColumns(ctx context.Context, db *sql.DB, schema string) (map[string][]string, error)
	DescribeTable(ctx context.Context, db *sql.DB, schema, table string) (Structure, error)
	ListIndexes(ctx context.Context, db *sql.DB, schema, table string) ([]Index, error)
	ListForeignKeys(ctx context.Context, db *sql.DB, schema, table string) ([]ForeignKey, error)

	FetchTableRows(ctx context.Context, db *sql.DB, opts RowsOpts) (RowsPage, error)

	PrimaryKey(ctx context.Context, db *sql.DB, schema, table string) ([]string, error)
	UpdateCell(ctx context.Context, db *sql.DB, schema, table string, pkCols []string, pkVals []any, col string, newVal any) (int64, error)
	InsertRow(ctx context.Context, db *sql.DB, schema, table string, cols []string, vals []any) (int64, error)
	DeleteRow(ctx context.Context, db *sql.DB, schema, table string, pkCols []string, pkVals []any) (int64, error)

	// KillQuery cancels a server-side running query identified by a
	// dialect-specific handle. MySQL uses CONNECTION_ID(); PG uses
	// pg_cancel_backend(pid); MSSQL uses KILL <spid>.
	KillQuery(ctx context.Context, db *sql.DB, connID int64) error
	// ConnectionIDQuery returns the SQL that yields the current
	// session's server-side connection id as a single int column.
	ConnectionIDQuery() string
}

// UnimplementedDialect is a test helper that panics on every method except
// the ones explicitly overridden by an embedder. Real dialects must NOT
// embed this — they are required to implement every method.
type UnimplementedDialect struct{}

func (UnimplementedDialect) Engine() Engine     { panic("unimplemented") }
func (UnimplementedDialect) DriverName() string { panic("unimplemented") }
func (UnimplementedDialect) BuildDSN(DSNInput) string {
	panic("unimplemented")
}
func (UnimplementedDialect) RegisterSSHDialer(SSHConfig) (string, func(), error) {
	panic("unimplemented")
}
func (UnimplementedDialect) QuoteIdent(string) string { panic("unimplemented") }
func (UnimplementedDialect) Placeholder(int) string   { panic("unimplemented") }
func (UnimplementedDialect) ClassifySQL(string) (Classified, error) {
	panic("unimplemented")
}
func (UnimplementedDialect) ListDatabases(context.Context, *sql.DB, bool) ([]string, error) {
	panic("unimplemented")
}
func (UnimplementedDialect) ListTables(context.Context, *sql.DB, string) ([]TableInfo, error) {
	panic("unimplemented")
}
func (UnimplementedDialect) ListSchemaColumns(context.Context, *sql.DB, string) (map[string][]string, error) {
	panic("unimplemented")
}
func (UnimplementedDialect) DescribeTable(context.Context, *sql.DB, string, string) (Structure, error) {
	panic("unimplemented")
}
func (UnimplementedDialect) ListIndexes(context.Context, *sql.DB, string, string) ([]Index, error) {
	panic("unimplemented")
}
func (UnimplementedDialect) ListForeignKeys(context.Context, *sql.DB, string, string) ([]ForeignKey, error) {
	panic("unimplemented")
}
func (UnimplementedDialect) FetchTableRows(context.Context, *sql.DB, RowsOpts) (RowsPage, error) {
	panic("unimplemented")
}
func (UnimplementedDialect) PrimaryKey(context.Context, *sql.DB, string, string) ([]string, error) {
	panic("unimplemented")
}
func (UnimplementedDialect) UpdateCell(context.Context, *sql.DB, string, string, []string, []any, string, any) (int64, error) {
	panic("unimplemented")
}
func (UnimplementedDialect) InsertRow(context.Context, *sql.DB, string, string, []string, []any) (int64, error) {
	panic("unimplemented")
}
func (UnimplementedDialect) DeleteRow(context.Context, *sql.DB, string, string, []string, []any) (int64, error) {
	panic("unimplemented")
}
func (UnimplementedDialect) KillQuery(context.Context, *sql.DB, int64) error {
	panic("unimplemented")
}
func (UnimplementedDialect) ConnectionIDQuery() string { panic("unimplemented") }
