package db

import (
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// PoolKey identifies a pooled connection. UserID isolates users so an admin
// switching identities never reuses another user's DB handle.
type PoolKey struct {
	UserID int64
	ConnID int64
}

// PoolConfig customizes Pool behavior. Open lets tests inject an in-memory
// driver; production callers leave it nil and the pool uses sql.Open with
// the dialect's DriverName.
type PoolConfig struct {
	IdleTimeout time.Duration
	Open        func(driver, dsn string) (*sql.DB, error)
}

type pooled struct {
	db        *sql.DB
	cacheKey  string
	sshCloser func()
	lastUsed  time.Time
}

// Pool caches *sql.DB handles keyed by PoolKey. It is engine-aware: each
// Get call carries a Dialect, so the pool can call dialect.BuildDSN and
// dialect.RegisterSSHDialer without consumers re-implementing engine logic.
type Pool struct {
	cfg PoolConfig
	mu  sync.Mutex
	m   map[PoolKey]*pooled
}

func NewPool(cfg PoolConfig) *Pool {
	if cfg.Open == nil {
		cfg.Open = func(driver, dsn string) (*sql.DB, error) {
			return sql.Open(driver, dsn)
		}
	}
	return &Pool{cfg: cfg, m: map[PoolKey]*pooled{}}
}

// Get returns or opens a *sql.DB for the key. SSH config, when non-zero,
// registers a per-tunnel dialer name with the dialect's driver and the DSN
// is regenerated to route through it.
func (p *Pool) Get(key PoolKey, d Dialect, in DSNInput, ssh SSHConfig) (*sql.DB, error) {
	cacheKey := d.Engine().String() + "|" + d.BuildDSN(in) + sshFingerprint(ssh)
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry, ok := p.m[key]; ok {
		if entry.cacheKey == cacheKey {
			entry.lastUsed = time.Now()
			return entry.db, nil
		}
		_ = entry.db.Close()
		if entry.sshCloser != nil {
			entry.sshCloser()
		}
		delete(p.m, key)
	}

	var sshCloser func()
	if !ssh.IsZero() {
		name, closer, err := d.RegisterSSHDialer(ssh)
		if err != nil {
			return nil, fmt.Errorf("ssh tunnel: %w", err)
		}
		sshCloser = closer
		in.Network = name
	}
	dsn := d.BuildDSN(in)

	db, err := p.cfg.Open(d.DriverName(), dsn)
	if err != nil {
		if sshCloser != nil {
			sshCloser()
		}
		return nil, fmt.Errorf("open: %w", err)
	}
	p.m[key] = &pooled{db: db, cacheKey: cacheKey, sshCloser: sshCloser, lastUsed: time.Now()}
	return db, nil
}

func (p *Pool) Evict(key PoolKey) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry, ok := p.m[key]; ok {
		_ = entry.db.Close()
		if entry.sshCloser != nil {
			entry.sshCloser()
		}
		delete(p.m, key)
	}
}

func (p *Pool) EvictUser(userID int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, entry := range p.m {
		if k.UserID == userID {
			_ = entry.db.Close()
			if entry.sshCloser != nil {
				entry.sshCloser()
			}
			delete(p.m, k)
		}
	}
}

func (p *Pool) Sweep(now time.Time) {
	if p.cfg.IdleTimeout == 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, entry := range p.m {
		if now.Sub(entry.lastUsed) >= p.cfg.IdleTimeout {
			_ = entry.db.Close()
			if entry.sshCloser != nil {
				entry.sshCloser()
			}
			delete(p.m, k)
		}
	}
}

// entryFor is test-only.
func (p *Pool) entryFor(key PoolKey) (*pooled, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.m[key]
	return e, ok
}

func sshFingerprint(s SSHConfig) string {
	if s.IsZero() {
		return ""
	}
	return fmt.Sprintf("|ssh=%s@%s:%d", s.User, s.Host, s.Port)
}
