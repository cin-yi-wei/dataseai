package mysql

import (
	"database/sql"
	"testing"
	"time"
)

func TestPool_LazyCreate_SameKeyReturnsSameDB(t *testing.T) {
	opens := 0
	p := NewPool(PoolConfig{
		IdleTimeout: 5 * time.Minute,
		Open: func(dsn string) (*sql.DB, error) {
			opens++
			return new(sql.DB), nil
		},
	})
	a, err := p.Get(PoolKey{UserID: 1, ConnID: 10}, "dsn1")
	if err != nil {
		t.Fatal(err)
	}
	b, err := p.Get(PoolKey{UserID: 1, ConnID: 10}, "dsn1")
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("same key should return same *sql.DB")
	}
	if opens != 1 {
		t.Fatalf("opens = %d, want 1", opens)
	}
}

func TestPool_DifferentKeysAreIsolated(t *testing.T) {
	opens := 0
	p := NewPool(PoolConfig{
		Open: func(dsn string) (*sql.DB, error) {
			opens++
			return new(sql.DB), nil
		},
	})
	_, _ = p.Get(PoolKey{UserID: 1, ConnID: 10}, "dsn1")
	_, _ = p.Get(PoolKey{UserID: 2, ConnID: 10}, "dsn2")
	if opens != 2 {
		t.Fatalf("opens = %d, want 2", opens)
	}
}

func TestPool_Evict(t *testing.T) {
	p := NewPool(PoolConfig{Open: func(dsn string) (*sql.DB, error) { return new(sql.DB), nil }})
	a, _ := p.Get(PoolKey{UserID: 1, ConnID: 10}, "dsn1")
	p.Evict(PoolKey{UserID: 1, ConnID: 10})
	b, _ := p.Get(PoolKey{UserID: 1, ConnID: 10}, "dsn1")
	if a == b {
		t.Fatal("evict should force re-open")
	}
}
