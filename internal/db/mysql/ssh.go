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

	"github.com/conray/dataseai/internal/db"
)

// RegisterSSHDialer dials the SSH bastion described by cfg, registers it as
// a go-sql-driver/mysql dialer, and returns the dialer name plus a closer
// the pool invokes on eviction. Callers should set DSNInput.Network to the
// returned name before BuildDSN so the next sql.Open routes through the
// tunnel.
//
// Returns an error on zero SSHConfig or auth/dial failure. The closer is
// safe to call multiple times.
func (MySQL) RegisterSSHDialer(cfg db.SSHConfig) (string, func(), error) {
	if cfg.IsZero() {
		return "", nil, fmt.Errorf("ssh: host/user required")
	}
	tun, err := openMySQLTunnel(cfg)
	if err != nil {
		return "", nil, err
	}
	return tun.name, func() { closeMySQLTunnel(tun.name) }, nil
}

type mysqlSSHTunnel struct {
	name   string
	client *ssh.Client
}

func (t *mysqlSSHTunnel) dial(ctx context.Context, addr string) (net.Conn, error) {
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

func (t *mysqlSSHTunnel) close() error {
	if t.client == nil {
		return nil
	}
	return t.client.Close()
}

var (
	mysqlSSHMu      sync.Mutex
	mysqlSSHTunnels = map[string]*mysqlSSHTunnel{}
	mysqlSSHCounter uint64
)

func nextMySQLDialerName() string {
	n := atomic.AddUint64(&mysqlSSHCounter, 1)
	return fmt.Sprintf("ssh-%d-%d", time.Now().UnixNano(), n)
}

func openMySQLTunnel(cfg db.SSHConfig) (*mysqlSSHTunnel, error) {
	if cfg.IsZero() {
		return nil, fmt.Errorf("ssh: host/user required")
	}
	port := cfg.Port
	if port == 0 {
		port = 22
	}
	var authMethod ssh.AuthMethod
	if cfg.PrivateKey != "" {
		var signer ssh.Signer
		var err error
		if cfg.KeyPassphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(cfg.PrivateKey), []byte(cfg.KeyPassphrase))
		} else {
			signer, err = ssh.ParsePrivateKey([]byte(cfg.PrivateKey))
		}
		if err != nil {
			return nil, fmt.Errorf("ssh key parse: %w", err)
		}
		authMethod = ssh.PublicKeys(signer)
	} else {
		authMethod = ssh.Password(cfg.Password)
	}
	sshCfg := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            []ssh.AuthMethod{authMethod},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}
	addr := fmt.Sprintf("%s:%d", cfg.Host, port)
	client, err := ssh.Dial("tcp", addr, sshCfg)
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s: %w", addr, err)
	}

	t := &mysqlSSHTunnel{name: nextMySQLDialerName(), client: client}

	mysqlSSHMu.Lock()
	mysqlSSHTunnels[t.name] = t
	mysqlSSHMu.Unlock()

	gomysql.RegisterDialContext(t.name, t.dial)
	return t, nil
}

func closeMySQLTunnel(name string) {
	mysqlSSHMu.Lock()
	t := mysqlSSHTunnels[name]
	delete(mysqlSSHTunnels, name)
	mysqlSSHMu.Unlock()
	if t != nil {
		_ = t.close()
	}
}
