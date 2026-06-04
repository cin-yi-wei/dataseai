package mysql

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	gomysql "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/ssh"
)

// SSHConfig describes how to reach the SSH bastion.
type SSHConfig struct {
	Host     string
	Port     int
	User     string
	Password string
}

// IsZero reports whether no SSH bastion is configured.
func (c SSHConfig) IsZero() bool {
	return c.Host == "" || c.User == ""
}

// sshTunnel keeps an SSH client around and serves as a custom dialer
// registered with the go-sql-driver/mysql driver.
type sshTunnel struct {
	name   string
	client *ssh.Client
}

func (t *sshTunnel) dial(ctx context.Context, addr string) (net.Conn, error) {
	type result struct {
		conn net.Conn
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		c, err := t.client.Dial("tcp", addr)
		ch <- result{conn: c, err: err}
	}()
	select {
	case r := <-ch:
		return r.conn, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (t *sshTunnel) close() error {
	if t.client == nil {
		return nil
	}
	return t.client.Close()
}

// Per-process registry of SSH dialers keyed by name. The go-sql-driver/mysql
// keeps its registered dialers in a global map, so we must avoid collisions
// and dispose of unused tunnels when the pool entry is evicted.
var (
	sshMu       sync.Mutex
	sshTunnels  = map[string]*sshTunnel{}
	sshCounter  uint64
)

// nextSSHDialerName picks a unique mysql dialer-name (used as the network
// portion of the DSN, e.g. `user:pwd@<name>(127.0.0.1:3306)/db`).
func nextSSHDialerName() string {
	n := atomic.AddUint64(&sshCounter, 1)
	return fmt.Sprintf("ssh-%d-%d", time.Now().UnixNano(), n)
}

// openSSHTunnel dials the SSH bastion, registers it as a mysql dialer under
// a fresh name, and returns the name so callers can build a DSN.
func openSSHTunnel(cfg SSHConfig) (*sshTunnel, error) {
	if cfg.IsZero() {
		return nil, fmt.Errorf("ssh: host/user required")
	}
	port := cfg.Port
	if port == 0 {
		port = 22
	}
	sshCfg := &ssh.ClientConfig{
		User: cfg.User,
		Auth: []ssh.AuthMethod{
			ssh.Password(cfg.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}
	addr := fmt.Sprintf("%s:%d", cfg.Host, port)
	client, err := ssh.Dial("tcp", addr, sshCfg)
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s: %w", addr, err)
	}

	t := &sshTunnel{name: nextSSHDialerName(), client: client}

	sshMu.Lock()
	sshTunnels[t.name] = t
	sshMu.Unlock()

	gomysql.RegisterDialContext(t.name, t.dial)
	return t, nil
}

// closeSSHTunnel disposes of a tunnel registered with openSSHTunnel.
// Note: there's no way to unregister a dialer from go-sql-driver/mysql, so
// the name remains taken for the process lifetime — but the underlying SSH
// client is closed and the entry is removed from our own map.
func closeSSHTunnel(name string) {
	sshMu.Lock()
	t := sshTunnels[name]
	delete(sshTunnels, name)
	sshMu.Unlock()
	if t != nil {
		_ = t.close()
	}
}
