package pg

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/crypto/ssh"

	"github.com/conray/dataseai/internal/db"
)

// RegisterSSHDialer opens an SSH bastion, builds a pgx ConnConfig from the
// provided DSNInput, attaches an SSH-backed DialFunc, registers it with
// stdlib, and returns the opaque pgx DSN reference. The pool feeds the
// reference to sql.Open("pgx", ref) verbatim — see BuildDSN's Network
// passthrough.
//
// Returns an error on zero SSHConfig or auth/dial failure. The closer is
// safe to call multiple times.
func (PG) RegisterSSHDialer(cfg db.SSHConfig, in db.DSNInput) (string, func(), error) {
	if cfg.IsZero() {
		return "", nil, fmt.Errorf("ssh: host/user required")
	}
	client, err := dialSSH(cfg)
	if err != nil {
		return "", nil, err
	}

	// Build the pgx ConnConfig from DSNInput. We start from a synthesized
	// key=value DSN (without the Network passthrough, since we're constructing
	// the real config here) and patch in the SSH dialer.
	connStr := (PG{}).buildBaseDSN(in)
	connCfg, err := pgx.ParseConfig(connStr)
	if err != nil {
		_ = client.Close()
		return "", nil, fmt.Errorf("pgx parse: %w", err)
	}
	connCfg.DialFunc = func(ctx context.Context, network, address string) (net.Conn, error) {
		return sshDial(ctx, client, address)
	}

	name := stdlib.RegisterConnConfig(connCfg)
	pgSSHRegistry.put(name, &pgSSHEntry{client: client})
	return name, func() { closePGSSH(name) }, nil
}

func sshDial(ctx context.Context, client *ssh.Client, address string) (net.Conn, error) {
	type result struct {
		conn net.Conn
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		c, err := client.Dial("tcp", address)
		ch <- result{conn: c, err: err}
	}()
	select {
	case r := <-ch:
		return r.conn, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func dialSSH(cfg db.SSHConfig) (*ssh.Client, error) {
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
	clientCfg := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            []ssh.AuthMethod{authMethod},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(port))
	client, err := ssh.Dial("tcp", addr, clientCfg)
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s: %w", addr, err)
	}
	return client, nil
}

type pgSSHEntry struct {
	client *ssh.Client
}

type pgSSHRegistryT struct {
	mu sync.Mutex
	m  map[string]*pgSSHEntry
}

func (r *pgSSHRegistryT) put(name string, e *pgSSHEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[name] = e
}

func (r *pgSSHRegistryT) take(name string) *pgSSHEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.m[name]
	delete(r.m, name)
	return e
}

var pgSSHRegistry = &pgSSHRegistryT{m: map[string]*pgSSHEntry{}}

func closePGSSH(name string) {
	e := pgSSHRegistry.take(name)
	if e == nil {
		return
	}
	stdlib.UnregisterConnConfig(name)
	_ = e.client.Close()
}
