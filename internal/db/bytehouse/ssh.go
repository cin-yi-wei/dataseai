package bytehouse

import (
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/conray/dataseai/internal/db"
)

// RegisterSSHDialer opens an SSH tunnel and starts a local TCP forwarder
// that proxies connections from 127.0.0.1:<random> to in.Host:in.Port via
// the SSH bastion. Returns the local "host:port" as the Network reference.
// BuildDSN substitutes that address into the DSN when Network is non-empty.
func (BH) RegisterSSHDialer(cfg db.SSHConfig, in db.DSNInput) (string, func(), error) {
	if cfg.IsZero() {
		return "", nil, fmt.Errorf("ssh: host/user required")
	}
	client, err := dialSSH(cfg)
	if err != nil {
		return "", nil, err
	}

	target := net.JoinHostPort(in.Host, strconv.Itoa(in.Port))

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = client.Close()
		return "", nil, fmt.Errorf("local listener: %w", err)
	}
	localAddr := listener.Addr().String()

	key := bhSSHReg.nextKey()
	entry := &bhSSHEntry{client: client, listener: listener}
	bhSSHReg.put(key, entry)

	// Accept loop: each local connection is forwarded through SSH.
	go func() {
		for {
			local, err := listener.Accept()
			if err != nil {
				return
			}
			go func(local net.Conn) {
				remote, err := client.Dial("tcp", target)
				if err != nil {
					_ = local.Close()
					return
				}
				go func() { _, _ = io.Copy(remote, local) }()
				go func() {
					_, _ = io.Copy(local, remote)
					_ = local.Close()
					_ = remote.Close()
				}()
			}(local)
		}
	}()

	return localAddr, func() { closeBHSSH(key) }, nil
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

type bhSSHEntry struct {
	client   *ssh.Client
	listener net.Listener
}

type bhSSHRegistry struct {
	mu sync.Mutex
	m  map[string]*bhSSHEntry
	n  uint64
}

func (r *bhSSHRegistry) put(key string, e *bhSSHEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[key] = e
}

func (r *bhSSHRegistry) take(key string) *bhSSHEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.m[key]
	delete(r.m, key)
	return e
}

func (r *bhSSHRegistry) nextKey() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.n++
	return strconv.FormatUint(r.n, 10)
}

var bhSSHReg = &bhSSHRegistry{m: map[string]*bhSSHEntry{}}

func closeBHSSH(key string) {
	e := bhSSHReg.take(key)
	if e == nil {
		return
	}
	_ = e.listener.Close()
	_ = e.client.Close()
}
