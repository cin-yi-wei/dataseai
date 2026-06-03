package mysql

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type PoolKey struct {
	UserID int64
	ConnID int64
}

type PoolConfig struct {
	IdleTimeout time.Duration                     // 0 = disabled
	Open        func(dsn string) (*sql.DB, error) // override for tests; nil uses sql.Open("mysql", dsn)
}

type pooled struct {
	db       *sql.DB
	dsn      string
	lastUsed time.Time
}

type Pool struct {
	cfg PoolConfig
	mu  sync.Mutex
	m   map[PoolKey]*pooled
}

func NewPool(cfg PoolConfig) *Pool {
	if cfg.Open == nil {
		cfg.Open = func(dsn string) (*sql.DB, error) {
			return sql.Open("mysql", dsn)
		}
	}
	return &Pool{cfg: cfg, m: map[PoolKey]*pooled{}}
}

func (p *Pool) Get(key PoolKey, dsn string) (*sql.DB, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry, ok := p.m[key]; ok {
		if entry.dsn == dsn {
			entry.lastUsed = time.Now()
			return entry.db, nil
		}
		// DSN changed (connection edited) — close stale entry and re-open
		_ = entry.db.Close()
		delete(p.m, key)
	}
	db, err := p.cfg.Open(dsn)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	p.m[key] = &pooled{db: db, dsn: dsn, lastUsed: time.Now()}
	return db, nil
}

func (p *Pool) Evict(key PoolKey) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry, ok := p.m[key]; ok {
		_ = entry.db.Close()
		delete(p.m, key)
	}
}

// EvictUser closes every pooled connection for a single user.
func (p *Pool) EvictUser(userID int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, entry := range p.m {
		if k.UserID == userID {
			_ = entry.db.Close()
			delete(p.m, k)
		}
	}
}

// Sweep closes any entry idle longer than IdleTimeout. No-op if IdleTimeout==0.
func (p *Pool) Sweep(now time.Time) {
	if p.cfg.IdleTimeout == 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, entry := range p.m {
		if now.Sub(entry.lastUsed) >= p.cfg.IdleTimeout {
			_ = entry.db.Close()
			delete(p.m, k)
		}
	}
}
