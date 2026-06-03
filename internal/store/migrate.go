package store

import (
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type migration struct {
	version int
	name    string
	sql     string
}

func loadMigrations() ([]migration, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, err
	}
	var ms []migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		base := strings.TrimSuffix(e.Name(), ".sql")
		// expect "NNNN_name"
		parts := strings.SplitN(base, "_", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("bad migration filename %q", e.Name())
		}
		v, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("bad version in %q: %w", e.Name(), err)
		}
		raw, err := migrationsFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return nil, err
		}
		ms = append(ms, migration{version: v, name: parts[1], sql: string(raw)})
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].version < ms[j].version })
	return ms, nil
}

func Migrate(db *sql.DB) error {
	ms, err := loadMigrations()
	if err != nil {
		return err
	}
	// Bootstrap: apply 0001 unconditionally if schema_migrations doesn't exist
	row := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='schema_migrations'")
	var tableName string
	if err := row.Scan(&tableName); err == sql.ErrNoRows {
		// no migrations table yet — apply 0001 which CREATEs it, then record
		if len(ms) == 0 || ms[0].version != 1 {
			return fmt.Errorf("migration 0001 missing")
		}
		if _, err := db.Exec(ms[0].sql); err != nil {
			return fmt.Errorf("apply 0001: %w", err)
		}
		if _, err := db.Exec("INSERT INTO schema_migrations(version) VALUES(?)", 1); err != nil {
			return fmt.Errorf("record 0001: %w", err)
		}
		ms = ms[1:]
	} else if err != nil {
		return err
	}
	// Apply each migration not yet applied
	applied := map[int]bool{1: true}
	rows, err := db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return err
	}
	for rows.Next() {
		var v int
		_ = rows.Scan(&v)
		applied[v] = true
	}
	_ = rows.Close()
	for _, m := range ms {
		if applied[m.version] {
			continue
		}
		if _, err := db.Exec(m.sql); err != nil {
			return fmt.Errorf("apply %04d_%s: %w", m.version, m.name, err)
		}
		if _, err := db.Exec("INSERT INTO schema_migrations(version) VALUES(?)", m.version); err != nil {
			return fmt.Errorf("record %d: %w", m.version, err)
		}
	}
	return nil
}
