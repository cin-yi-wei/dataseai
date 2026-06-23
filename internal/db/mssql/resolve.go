package mssql

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

// tableLoc is a database+schema where a bare table name was found.
type tableLoc struct {
	DB     string
	Schema string
}

var invalidObjectRe = regexp.MustCompile(`(?i)invalid object name '([^']+)'`)

// parseInvalidObject pulls the object name out of a SQL Server
// "Invalid object name 'X'" error. Returns "" if the error isn't that shape
// or the name is already qualified (contains a dot) — we only auto-resolve
// bare, unqualified references and leave anything the user qualified alone.
func parseInvalidObject(err error) string {
	if err == nil {
		return ""
	}
	m := invalidObjectRe.FindStringSubmatch(err.Error())
	if m == nil {
		return ""
	}
	name := strings.Trim(m[1], "[]")
	if strings.ContainsAny(name, ".[]") {
		return ""
	}
	return name
}

// userDatabases lists online, non-system databases to scan.
func userDatabases(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		"SELECT name FROM sys.databases WHERE database_id > 4 AND state = 0 ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// locateTable finds every database+schema whose tables include name
// (case-insensitive per server collation). Uses 3-part INFORMATION_SCHEMA
// references so it doesn't disturb the pooled connection's current database.
// Databases it can't read (offline / no permission) are skipped.
func locateTable(ctx context.Context, db *sql.DB, name string) []tableLoc {
	dbs, err := userDatabases(ctx, db)
	if err != nil {
		return nil
	}
	var out []tableLoc
	seen := map[string]bool{}
	for _, d := range dbs {
		q := "SELECT TABLE_SCHEMA FROM " + (MSSQL{}).QuoteIdent(d) +
			".INFORMATION_SCHEMA.TABLES WHERE TABLE_NAME = @p1"
		rows, err := db.QueryContext(ctx, q, name)
		if err != nil {
			continue
		}
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err == nil {
				key := d + "." + s
				if !seen[key] {
					seen[key] = true
					out = append(out, tableLoc{DB: d, Schema: s})
				}
			}
		}
		rows.Close()
	}
	return out
}

// qualifyName rewrites bare, unqualified references to name in statement into
// a fully-qualified [DB].[schema].[name]. It only touches occurrences that
// aren't already part of a qualified name (not preceded by '.', a word char,
// or '['), so aliases and already-qualified refs are left intact. Go's
// regexp does not rescan the replacement, so this is a single safe pass.
func qualifyName(statement, name string, loc tableLoc) string {
	full := (MSSQL{}).QuoteIdent(loc.DB) + "." + (MSSQL{}).QuoteIdent(loc.Schema) + "." + (MSSQL{}).QuoteIdent(name)
	re := regexp.MustCompile(`(?i)(^|[^.\w\[])` + regexp.QuoteMeta(name) + `\b`)
	repl := "${1}" + strings.ReplaceAll(full, "$", "$$")
	return re.ReplaceAllString(statement, repl)
}

// ambiguousError reports that a bare name resolved to more than one place and
// asks the caller to qualify it.
func ambiguousError(name string, locs []tableLoc) error {
	paths := make([]string, 0, len(locs))
	for _, l := range locs {
		paths = append(paths, fmt.Sprintf("[%s].[%s].[%s]", l.DB, l.Schema, name))
	}
	return fmt.Errorf("ambiguous object name %q — found in %s; qualify it explicitly", name, strings.Join(paths, ", "))
}
