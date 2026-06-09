package mssql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	mssqldrv "github.com/microsoft/go-mssqldb"
	"github.com/microsoft/go-mssqldb/msdsn"
	"golang.org/x/crypto/ssh"

	"github.com/conray/dataseai/internal/db"
)

// driverName is used for both normal and SSH connections. The SSH wrapper
// driver (registered once as this name) delegates non-SSH DSNs to the
// underlying go-mssqldb driver transparently.
const driverName = "sqlserver-ssh"

const sshPrefix = "mssql-ssh:"

// mssqlSSHDriver is a database/sql driver that handles two cases:
//   - DSN prefixed with sshPrefix → look up pre-built Connector in registry
//   - anything else → delegate to go-mssqldb Driver directly
type mssqlSSHDriver struct{}

func (d mssqlSSHDriver) Open(name string) (driver.Conn, error) {
	if len(name) > len(sshPrefix) && name[:len(sshPrefix)] == sshPrefix {
		key := name[len(sshPrefix):]
		e := sshReg.get(key)
		if e == nil {
			return nil, fmt.Errorf("mssql-ssh: no connector for %q", key)
		}
		return e.connector.Connect(context.Background())
	}
	return (&mssqldrv.Driver{}).Open(name)
}

var registerOnce sync.Once

func registerSSHDriver() {
	registerOnce.Do(func() {
		// sql.Register panics on duplicate; guard with Once.
		// We import _ "github.com/microsoft/go-mssqldb" in dialect.go which
		// registers "sqlserver". We register our wrapper as "sqlserver-ssh".
		sql.Register(driverName, mssqlSSHDriver{})
	})
}

// RegisterSSHDialer opens an SSH tunnel to cfg.Host, builds a Connector
// with the SSH dialer attached, and returns an opaque reference DSN for use
// as in.Network. The pool feeds that reference to sql.Open(driverName, ref).
func (MSSQL) RegisterSSHDialer(cfg db.SSHConfig, in db.DSNInput) (string, func(), error) {
	if cfg.IsZero() {
		return "", nil, fmt.Errorf("ssh: host/user required")
	}
	client, err := dialSSH(cfg)
	if err != nil {
		return "", nil, err
	}

	msCfg := msdsn.Config{
		Host:     in.Host,
		Port:     uint64(in.Port),
		User:     in.Username,
		Password: in.Password,
		Database: in.DefaultDB,
	}
	switch in.TLS {
	case "disabled":
		msCfg.Encryption = msdsn.EncryptionDisabled
		msCfg.TrustServerCertificate = true
	case "skip-verify":
		msCfg.Encryption = msdsn.EncryptionRequired
		msCfg.TrustServerCertificate = true
	case "required":
		msCfg.Encryption = msdsn.EncryptionRequired
	default:
		msCfg.Encryption = msdsn.EncryptionOff
	}
	msCfg.DialTimeout = 30 * time.Second

	connector := mssqldrv.NewConnectorConfig(msCfg)
	connector.Dialer = &sshDialer{client: client}

	key := generateKey()
	sshReg.put(key, &sshEntry{connector: connector, client: client})
	ref := sshPrefix + key

	return ref, func() { closeSSH(key) }, nil
}

// sshDialer implements mssql.Dialer by routing connections through an SSH tunnel.
type sshDialer struct {
	client *ssh.Client
}

func (d *sshDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	type result struct {
		conn net.Conn
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		c, err := d.client.Dial("tcp", addr)
		ch <- result{c, err}
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

type sshEntry struct {
	connector *mssqldrv.Connector
	client    *ssh.Client
}

type sshRegistry struct {
	mu sync.Mutex
	m  map[string]*sshEntry
	n  uint64
}

func (r *sshRegistry) put(key string, e *sshEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[key] = e
}

func (r *sshRegistry) get(key string) *sshEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.m[key]
}

func (r *sshRegistry) take(key string) *sshEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.m[key]
	delete(r.m, key)
	return e
}

func (r *sshRegistry) nextKey() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.n++
	return strconv.FormatUint(r.n, 10)
}

var sshReg = &sshRegistry{m: map[string]*sshEntry{}}

func generateKey() string { return sshReg.nextKey() }

func closeSSH(key string) {
	e := sshReg.take(key)
	if e == nil {
		return
	}
	_ = e.client.Close()
}
