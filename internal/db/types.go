// Package db defines the engine-agnostic surface every supported relational
// database must implement. The dialect interface lives in dialect.go; this
// file collects the value types those methods produce or consume.
package db

import (
	"errors"
	"fmt"
)

// Engine names a database engine supported by the platform.
type Engine string

const (
	EngineMySQL     Engine = "mysql"
	EnginePostgres  Engine = "postgres"
	EngineMSSQL     Engine = "mssql"
	EngineBytehouse Engine = "bytehouse"
	EngineSQLite      Engine = "sqlite"
	EngineMariaDB     Engine = "mariadb"
	EngineTiDB        Engine = "tidb"
	EngineCockroachDB Engine = "cockroachdb"
	EngineRedshift    Engine = "redshift"
	EngineSingleStore Engine = "singlestore"
	EngineDuckDB      Engine = "duckdb"
	EngineSnowflake   Engine = "snowflake"
	EngineClickHouse  Engine = "clickhouse"
	EnginePlanetScale Engine = "planetscale"
)

func (e Engine) String() string { return string(e) }

// ParseEngine normalizes a stored string into a known Engine. Returns an
// error for empty or unrecognized values so callers can decide how to
// surface the failure (HTTP 400, migration default, etc).
func ParseEngine(s string) (Engine, error) {
	switch Engine(s) {
	case EngineMySQL:
		return EngineMySQL, nil
	case EnginePostgres:
		return EnginePostgres, nil
	case EngineMSSQL:
		return EngineMSSQL, nil
	case EngineBytehouse:
		return EngineBytehouse, nil
	case EngineSQLite:
		return EngineSQLite, nil
	case EngineMariaDB:
		return EngineMariaDB, nil
	case EngineTiDB:
		return EngineTiDB, nil
	case EngineCockroachDB:
		return EngineCockroachDB, nil
	case EngineRedshift:
		return EngineRedshift, nil
	case EngineSingleStore:
		return EngineSingleStore, nil
	case EngineDuckDB:
		return EngineDuckDB, nil
	case EngineSnowflake:
		return EngineSnowflake, nil
	case EngineClickHouse:
		return EngineClickHouse, nil
	case EnginePlanetScale:
		return EnginePlanetScale, nil
	default:
		return "", fmt.Errorf("unknown engine %q", s)
	}
}

// DSNInput holds the bits a dialect needs to build its driver-specific DSN.
// Not every dialect uses every field (e.g. TLS string maps differently per
// driver), but the shape is shared so consumers don't branch on engine.
type DSNInput struct {
	Host      string
	Port      int
	Username  string
	Password  string
	DefaultDB string
	TLS       string // "disabled" | "preferred" | "required" | "skip-verify"
	Network   string // SSH dialer name override; empty = engine default ("tcp")
}

// SSHConfig describes how to reach an SSH bastion.
type SSHConfig struct {
	Host          string
	Port          int
	User          string
	Password      string
	PrivateKey    string
	KeyPassphrase string
}

func (c SSHConfig) IsZero() bool { return c.Host == "" || c.User == "" }

// Column describes one column returned by Dialect.DescribeTable.
type Column struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
	Default  string `json:"default"`
	Extra    string `json:"extra"`
	Comment  string `json:"comment"`
	Key      string `json:"key"`
}

// Structure groups columns and a CREATE-like representation. Dialects that
// have no SHOW CREATE TABLE equivalent should synthesize a best-effort
// CREATE TABLE string from the columns for parity.
type Structure struct {
	Columns   []Column `json:"columns"`
	CreateSQL string   `json:"create_sql"`
}

type Index struct {
	Name      string   `json:"name"`
	Columns   []string `json:"columns"`
	Unique    bool     `json:"unique"`
	IndexType string   `json:"index_type"`
}

type ForeignKey struct {
	Name      string `json:"name"`
	Column    string `json:"column"`
	RefTable  string `json:"ref_table"`
	RefColumn string `json:"ref_column"`
	OnDelete  string `json:"on_delete"`
	OnUpdate  string `json:"on_update"`
}

type TableInfo struct {
	Name    string `json:"name"`
	RowsEst int64  `json:"rows_est"`
	SizeMB  int64  `json:"size_mb"`
}

type Filter struct {
	Column   string `json:"column"`
	Operator string `json:"operator"`
	Value    string `json:"value"`
}

type RowsOpts struct {
	Schema  string
	Table   string
	Page    int
	PerPage int
	SortCol string
	SortDir string
	Filters []Filter
}

type RowsPage struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
	Total   int64    `json:"total"`
	Page    int      `json:"page"`
	PerPage int      `json:"per_page"`
}

// Op classifies a parsed SQL statement.
type Op string

const (
	OpSelect    Op = "SELECT"
	OpInsert    Op = "INSERT"
	OpUpdate    Op = "UPDATE"
	OpDelete    Op = "DELETE"
	OpTruncate  Op = "TRUNCATE"
	OpDDL       Op = "DDL"
	OpForbidden Op = "FORBIDDEN"
	OpReadMeta  Op = "READMETA"
	OpUnknown   Op = "UNKNOWN"
)

type Classified struct {
	Op    Op
	DB    string
	Table string
	Multi bool
}

// MaxRowsPerPage is the absolute upper bound on a paginated table-rows
// response. Dialects must clamp `PerPage` to at most this value.
const MaxRowsPerPage = 10000

// ErrNoPrimaryKey signals a table has no primary key so edit-via-PK is
// not possible. Returned by Dialect.UpdateCell / DeleteRow.
var ErrNoPrimaryKey = errors.New("table has no primary key, edit disabled")
