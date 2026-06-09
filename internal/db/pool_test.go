package db

import (
	"database/sql"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type fakeDialect struct {
	UnimplementedDialect
}

func (f *fakeDialect) BuildDSN(in DSNInput) string {
	return in.Host
}
func (f *fakeDialect) Engine() Engine     { return Engine("fake") }
func (f *fakeDialect) DriverName() string { return "sqlite3" } // unused; pool uses cfg.Open

func TestPoolReusesEntryWhenKeyAndDSNMatch(t *testing.T) {
	opens := int32(0)
	p := NewPool(PoolConfig{
		Open: func(driver, dsn string) (*sql.DB, error) {
			atomic.AddInt32(&opens, 1)
			return sql.Open("sqlite3", ":memory:")
		},
	})
	d := &fakeDialect{}
	key := PoolKey{UserID: 1, ConnID: 1}
	a, err := p.Get(key, d, DSNInput{Host: "h"}, SSHConfig{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := p.Get(key, d, DSNInput{Host: "h"}, SSHConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("expected pool to reuse entry")
	}
	if atomic.LoadInt32(&opens) != 1 {
		t.Fatalf("opens = %d, want 1", opens)
	}
}

func TestPoolReopensWhenDSNChanges(t *testing.T) {
	opens := int32(0)
	p := NewPool(PoolConfig{
		Open: func(driver, dsn string) (*sql.DB, error) {
			atomic.AddInt32(&opens, 1)
			return sql.Open("sqlite3", ":memory:")
		},
	})
	d := &fakeDialect{}
	key := PoolKey{UserID: 1, ConnID: 1}
	if _, err := p.Get(key, d, DSNInput{Host: "h1"}, SSHConfig{}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Get(key, d, DSNInput{Host: "h2"}, SSHConfig{}); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&opens) != 2 {
		t.Fatalf("opens = %d, want 2", opens)
	}
}

func TestPoolEvict(t *testing.T) {
	p := NewPool(PoolConfig{Open: func(driver, dsn string) (*sql.DB, error) {
		db, _ := sql.Open("sqlite3", ":memory:")
		return db, nil
	}})
	d := &fakeDialect{}
	key := PoolKey{UserID: 1, ConnID: 2}
	if _, err := p.Get(key, d, DSNInput{Host: "h"}, SSHConfig{}); err != nil {
		t.Fatal(err)
	}
	p.Evict(key)
	if _, ok := p.entryFor(key); ok {
		t.Fatal("evict did not remove entry")
	}
}

func TestPoolSweepIdle(t *testing.T) {
	p := NewPool(PoolConfig{IdleTimeout: 10 * time.Millisecond, Open: func(driver, dsn string) (*sql.DB, error) {
		db, _ := sql.Open("sqlite3", ":memory:")
		return db, nil
	}})
	d := &fakeDialect{}
	key := PoolKey{UserID: 1, ConnID: 3}
	if _, err := p.Get(key, d, DSNInput{Host: "h"}, SSHConfig{}); err != nil {
		t.Fatal(err)
	}
	p.Sweep(time.Now().Add(time.Second))
	if _, ok := p.entryFor(key); ok {
		t.Fatal("sweep did not evict idle entry")
	}
}

func TestPoolOpenError(t *testing.T) {
	want := errors.New("boom")
	p := NewPool(PoolConfig{Open: func(driver, dsn string) (*sql.DB, error) { return nil, want }})
	d := &fakeDialect{}
	_, err := p.Get(PoolKey{UserID: 1, ConnID: 4}, d, DSNInput{Host: "h"}, SSHConfig{})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want wrap of boom", err)
	}
}
